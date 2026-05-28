//go:build vxlan_integration
// +build vxlan_integration

// VXLAN multi-node integration test against a real PostgreSQL database.
//
// This test simulates two compute nodes coordinating VXLAN state through
// a shared database. It uses stub-mode VXLANManager + NamespaceManager
// (no netlink, no root) — the subject under test is the coordination
// layer, not kernel-side networking. The networking primitives are
// covered by the bash test in test/vxlan_multinode_test.sh and by
// component-level tests under pkg/networking.
//
// Requires:
//   - reachable PostgreSQL with the o3k schema (migrations are reset
//     and re-applied on every run, so the database is wiped clean)
//   - migrations directory at ../../migrations relative to this file
//
// Environment:
//   O3K_TEST_DB_URL  postgres connection string (required; test skips if unset)
//
// Run:
//   O3K_TEST_DB_URL=postgres://lightstack:secret@localhost:5432/lightstack_test?sslmode=disable \
//     go test -tags vxlan_integration -v -count=1 -timeout 5m ./internal/neutron/...
//
// Closes Phase 2 of docs/kimi-analyse-for-completion.md by proving the
// FDB-sync code path works end-to-end across simulated nodes.

package neutron

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/compute"
	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/cobaltcore-dev/o3k/pkg/networking"
	"github.com/google/uuid"
)

const (
	testHeartbeatInterval = 1 * time.Second
	testPollInterval      = 100 * time.Millisecond
	testVNIRangeStart     = 1000
	testVNIRangeEnd       = 2000
	defaultDomainID       = "00000000-0000-0000-0000-000000000100"
)

// testEnv reads the test database URL and locates the migrations
// directory. Skips the test if the env var is unset so `go test ./...`
// without the integration tag stays green.
func testEnv(t *testing.T) (dbURL, migrationsPath string) {
	t.Helper()
	dbURL = os.Getenv("O3K_TEST_DB_URL")
	if dbURL == "" {
		t.Skip("O3K_TEST_DB_URL not set; skipping VXLAN multi-node integration test")
	}
	// internal/neutron/<this file> → migrations are at ../../migrations
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	migrationsPath = filepath.Join(wd, "..", "..", "migrations")
	if _, err := os.Stat(migrationsPath); err != nil {
		t.Fatalf("migrations dir %s not readable: %v", migrationsPath, err)
	}
	return dbURL, migrationsPath
}

// resetSchema drops and re-applies all migrations so each test starts
// from a known empty state. We do this once per test (not per subtest)
// to keep wall time reasonable.
func resetSchema(t *testing.T, dbURL, migrationsPath string) {
	t.Helper()
	if err := database.MigrateReset(dbURL, migrationsPath); err != nil {
		t.Fatalf("MigrateReset: %v", err)
	}
}

// setupDB connects pgx and returns the DBIF. Caller must Close.
func setupDB(t *testing.T, dbURL string) database.DBIF {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := database.ConnectSimple(ctx, dbURL, 8); err != nil {
		t.Fatalf("database.Connect: %v", err)
	}
	return database.DB
}

// node bundles a simulated compute node's coordination components.
type node struct {
	hostname  string
	tunnelIP  string
	registry  *compute.NodeRegistry
	vxlanMgr  *networking.VXLANManager
	nsMgr     *networking.NetworkNamespaceManager
	coord     *VXLANCoordinator
}

func newNode(t *testing.T, hostname, tunnelIP string, db database.DBIF) *node {
	t.Helper()
	reg := compute.NewNodeRegistryForTest(
		uuid.NewString(),
		hostname,
		tunnelIP,
		testHeartbeatInterval,
		db,
	)
	vmgr := networking.NewVXLANManager("stub", 4789)
	nsmgr := networking.NewNetworkNamespaceManager("stub")
	coord := NewVXLANCoordinator(
		vmgr, reg, nsmgr,
		testPollInterval,
		testVNIRangeStart, testVNIRangeEnd,
	)
	coord.setDB(db)
	return &node{
		hostname: hostname,
		tunnelIP: tunnelIP,
		registry: reg,
		vxlanMgr: vmgr,
		nsMgr:    nsmgr,
		coord:    coord,
	}
}

// seedProject inserts a project row tied to the seeded Default domain
// so VXLAN networks created against it satisfy the FK on
// networks.project_id.
func seedProject(t *testing.T, db database.DBIF, ctx context.Context) string {
	t.Helper()
	projectID := uuid.NewString()
	_, err := db.Exec(ctx, `
		INSERT INTO projects (id, name, description, enabled, domain_id)
		VALUES ($1, $2, $3, $4, $5)
	`, projectID, "vxlan-test-"+projectID[:8], "vxlan multi-node test project", true, defaultDomainID)
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	return projectID
}

// seedVXLANNetwork inserts a VXLAN-typed network with a fresh UUID.
// VNI is allocated lazily by the coordinator on its next sync.
func seedVXLANNetwork(t *testing.T, db database.DBIF, ctx context.Context, projectID string) string {
	t.Helper()
	networkID := uuid.NewString()
	_, err := db.Exec(ctx, `
		INSERT INTO networks (id, name, project_id, network_type, status)
		VALUES ($1, $2, $3, 'vxlan', 'ACTIVE')
	`, networkID, "net-"+networkID[:8], projectID)
	if err != nil {
		t.Fatalf("seed VXLAN network: %v", err)
	}
	return networkID
}

// seedPort inserts a port row so vxlan_fdb_entries.port_id has a valid
// FK target. The FDB sync test would otherwise fail with SQLSTATE 23503
// because DistributeFDBEntry inserts into vxlan_fdb_entries directly.
func seedPort(t *testing.T, db database.DBIF, ctx context.Context, networkID, projectID, mac string) string {
	t.Helper()
	portID := uuid.NewString()
	_, err := db.Exec(ctx, `
		INSERT INTO ports (id, name, network_id, project_id, mac_address, status)
		VALUES ($1, $2, $3, $4, $5, 'ACTIVE')
	`, portID, "port-"+portID[:8], networkID, projectID, mac)
	if err != nil {
		t.Fatalf("seed port: %v", err)
	}
	return portID
}

// TestVXLANMultiNode_TwoNodesRegister verifies both nodes appear in
// ListActiveNodes after registration and heartbeat.
func TestVXLANMultiNode_TwoNodesRegister(t *testing.T) {
	dbURL, mig := testEnv(t)
	resetSchema(t, dbURL, mig)
	db := setupDB(t, dbURL)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	n1 := newNode(t, "test-node-1", "10.99.0.1", db)
	n2 := newNode(t, "test-node-2", "10.99.0.2", db)

	if err := n1.registry.RegisterNode(ctx); err != nil {
		t.Fatalf("n1 register: %v", err)
	}
	if err := n2.registry.RegisterNode(ctx); err != nil {
		t.Fatalf("n2 register: %v", err)
	}

	nodes, err := n1.registry.ListActiveNodes(ctx)
	if err != nil {
		t.Fatalf("ListActiveNodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 active nodes, got %d (%+v)", len(nodes), nodes)
	}

	seen := map[string]bool{}
	for _, n := range nodes {
		seen[n.Hostname] = true
	}
	if !seen["test-node-1"] || !seen["test-node-2"] {
		t.Errorf("missing expected hostnames: %+v", seen)
	}
}

// TestVXLANMultiNode_VNIAllocationIsUnique creates two VXLAN networks
// and verifies each gets a distinct VNI in the configured range.
func TestVXLANMultiNode_VNIAllocationIsUnique(t *testing.T) {
	dbURL, mig := testEnv(t)
	resetSchema(t, dbURL, mig)
	db := setupDB(t, dbURL)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	n1 := newNode(t, "test-node-1", "10.99.0.1", db)
	if err := n1.registry.RegisterNode(ctx); err != nil {
		t.Fatalf("register: %v", err)
	}

	projectID := seedProject(t, db, ctx)
	netA := seedVXLANNetwork(t, db, ctx, projectID)
	netB := seedVXLANNetwork(t, db, ctx, projectID)

	vniA, err := n1.coord.GetVNI(ctx, netA)
	if err != nil {
		t.Fatalf("GetVNI A: %v", err)
	}
	vniB, err := n1.coord.GetVNI(ctx, netB)
	if err != nil {
		t.Fatalf("GetVNI B: %v", err)
	}

	if vniA == vniB {
		t.Errorf("expected distinct VNIs, got %d for both", vniA)
	}
	for _, v := range []int{vniA, vniB} {
		if v < testVNIRangeStart || v > testVNIRangeEnd {
			t.Errorf("VNI %d outside range [%d,%d]", v, testVNIRangeStart, testVNIRangeEnd)
		}
	}

	// Idempotency: second call returns the already-allocated VNI.
	vniA2, err := n1.coord.GetVNI(ctx, netA)
	if err != nil {
		t.Fatalf("GetVNI A repeat: %v", err)
	}
	if vniA2 != vniA {
		t.Errorf("VNI for net A changed across calls: %d → %d", vniA, vniA2)
	}
}

// TestVXLANMultiNode_FDBSyncBetweenNodes is the core scenario from the
// Kimi audit: a port created on node 1 should produce an FDB entry that
// node 2's syncPorts loop picks up and feeds to its VXLANManager.
func TestVXLANMultiNode_FDBSyncBetweenNodes(t *testing.T) {
	dbURL, mig := testEnv(t)
	resetSchema(t, dbURL, mig)
	db := setupDB(t, dbURL)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	n1 := newNode(t, "test-node-1", "10.99.0.1", db)
	n2 := newNode(t, "test-node-2", "10.99.0.2", db)
	for _, n := range []*node{n1, n2} {
		if err := n.registry.RegisterNode(ctx); err != nil {
			t.Fatalf("%s register: %v", n.hostname, err)
		}
	}

	projectID := seedProject(t, db, ctx)
	netID := seedVXLANNetwork(t, db, ctx, projectID)

	// Node 1 advertises a port. The vtep_ip recorded in the row
	// should match n1's tunnel IP.
	mac := "fa:16:3e:aa:bb:01"
	portID := seedPort(t, db, ctx, netID, projectID, mac)
	if err := n1.coord.DistributeFDBEntry(ctx, netID, portID, mac); err != nil {
		t.Fatalf("DistributeFDBEntry: %v", err)
	}

	// Verify the row landed with n1's tunnel IP.
	var vtepIP string
	if err := db.QueryRow(ctx,
		`SELECT vtep_ip FROM vxlan_fdb_entries WHERE port_id = $1`, portID,
	).Scan(&vtepIP); err != nil {
		t.Fatalf("read FDB row: %v", err)
	}
	if vtepIP != n1.tunnelIP {
		t.Fatalf("FDB row vtep_ip = %q, want %q", vtepIP, n1.tunnelIP)
	}

	// Run n2's coordinator briefly so syncPorts pulls the row and
	// asks its (stub) VXLANManager to install the FDB entry.
	runCtx, runCancel := context.WithTimeout(ctx, 2*time.Second)
	defer runCancel()
	go n2.coord.Start(runCtx)
	// Give the coordinator at least a few poll cycles.
	time.Sleep(500 * time.Millisecond)
	n2.coord.Stop()

	// The stub VXLANManager records FDB additions in its in-memory
	// map. Confirm node 2 saw the remote MAC pointing to node 1.
	n2FDB, err := n2.vxlanMgr.ListFDBEntries(netID)
	if err != nil {
		t.Fatalf("n2 ListFDBEntries: %v", err)
	}
	if remoteIP, ok := n2FDB[mac]; !ok {
		t.Errorf("n2 stub VXLANManager has no FDB entry for net=%s mac=%s after sync (entries: %+v)", netID, mac, n2FDB)
	} else if remoteIP != n1.tunnelIP {
		t.Errorf("n2 FDB entry for %s points to %s, want %s", mac, remoteIP, n1.tunnelIP)
	}

	// And node 1 must NOT install its own MAC as a remote FDB —
	// syncPorts skips local-VTEP entries. n1's coordinator never ran
	// here, but assert the invariant explicitly so a future change
	// that auto-starts the coordinator in newNode() doesn't slip past.
	n1FDB, err := n1.vxlanMgr.ListFDBEntries(netID)
	if err != nil {
		t.Fatalf("n1 ListFDBEntries: %v", err)
	}
	if _, ok := n1FDB[mac]; ok {
		t.Errorf("n1 unexpectedly installed FDB entry for its own port (should skip local VTEP)")
	}
}

// TestVXLANMultiNode_StoppedHeartbeatDeactivates verifies a node whose
// heartbeat lapses past 2× the heartbeat interval drops out of
// ListActiveNodes. This is what triggers FDB cleanup on surviving nodes.
func TestVXLANMultiNode_StoppedHeartbeatDeactivates(t *testing.T) {
	dbURL, mig := testEnv(t)
	resetSchema(t, dbURL, mig)
	db := setupDB(t, dbURL)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	n1 := newNode(t, "test-node-1", "10.99.0.1", db)
	n2 := newNode(t, "test-node-2", "10.99.0.2", db)
	for _, n := range []*node{n1, n2} {
		if err := n.registry.RegisterNode(ctx); err != nil {
			t.Fatalf("%s register: %v", n.hostname, err)
		}
	}

	// Both visible immediately after registration.
	nodes, err := n1.registry.ListActiveNodes(ctx)
	if err != nil {
		t.Fatalf("ListActiveNodes initial: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes after register, got %d", len(nodes))
	}

	// Backdate n2's heartbeat to 3× the interval ago — past the
	// 2× threshold ListActiveNodes uses.
	stale := time.Now().Add(-3 * testHeartbeatInterval)
	if _, err := db.Exec(ctx,
		`UPDATE compute_nodes SET last_heartbeat = $1 WHERE hostname = $2`,
		stale, "test-node-2",
	); err != nil {
		t.Fatalf("backdate heartbeat: %v", err)
	}

	nodes, err = n1.registry.ListActiveNodes(ctx)
	if err != nil {
		t.Fatalf("ListActiveNodes after backdate: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 active node after backdating n2, got %d (%+v)", len(nodes), nodes)
	}
	if nodes[0].Hostname != "test-node-1" {
		t.Errorf("expected n1 surviving, got %q", nodes[0].Hostname)
	}
}
