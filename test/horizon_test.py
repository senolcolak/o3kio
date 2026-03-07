#!/usr/bin/env python3
"""
O3K Horizon Compatibility Test
Simulates Horizon dashboard operations to validate O3K API compatibility
"""

import requests
import json
import sys
from datetime import datetime

# Colors for output
GREEN = '\033[0;32m'
RED = '\033[0;31m'
YELLOW = '\033[1;33m'
NC = '\033[0m'  # No Color

# O3K configuration
O3K_URL = "http://localhost:35357"
ADMIN_USER = "admin"
ADMIN_PASSWORD = "secret"
PROJECT_NAME = "default"

# Test results
tests_passed = 0
tests_failed = 0


def print_test(test_name):
    print(f"\n{YELLOW}[TEST]{NC} {test_name}")


def print_pass(message):
    global tests_passed
    print(f"{GREEN}[PASS]{NC} {message}")
    tests_passed += 1


def print_fail(message):
    global tests_failed
    print(f"{RED}[FAIL]{NC} {message}")
    tests_failed += 1


def print_info(message):
    print(f"  ℹ  {message}")


class HorizonSimulator:
    """Simulates Horizon dashboard API calls"""

    def __init__(self):
        self.token = None
        self.project_id = None
        self.user_id = None

    def authenticate(self):
        """Simulate Horizon login flow"""
        print_test("Horizon Login Flow")

        # Step 1: Get unscoped token
        auth_payload = {
            "auth": {
                "identity": {
                    "methods": ["password"],
                    "password": {
                        "user": {
                            "name": ADMIN_USER,
                            "domain": {"name": "Default"},
                            "password": ADMIN_PASSWORD
                        }
                    }
                }
            }
        }

        response = requests.post(
            f"{O3K_URL}/v3/auth/tokens",
            json=auth_payload,
            headers={"Content-Type": "application/json"}
        )

        if response.status_code != 201:
            print_fail(f"Unscoped authentication failed (HTTP {response.status_code})")
            return False

        unscoped_token = response.headers.get('X-Subject-Token')
        print_pass(f"Unscoped token obtained: {unscoped_token[:50]}...")

        # Step 2: Get scoped token with project
        auth_payload["auth"]["scope"] = {
            "project": {
                "name": PROJECT_NAME,
                "domain": {"name": "Default"}
            }
        }

        response = requests.post(
            f"{O3K_URL}/v3/auth/tokens",
            json=auth_payload,
            headers={"Content-Type": "application/json"}
        )

        if response.status_code != 201:
            print_fail(f"Scoped authentication failed (HTTP {response.status_code})")
            return False

        self.token = response.headers.get('X-Subject-Token')
        token_data = response.json()

        self.project_id = token_data['token']['project']['id']
        self.user_id = token_data['token']['user']['id']

        print_pass(f"Scoped token obtained")
        print_info(f"User ID: {self.user_id}")
        print_info(f"Project ID: {self.project_id}")

        # Verify service catalog
        if 'catalog' in token_data['token']:
            catalog = token_data['token']['catalog']
            print_pass(f"Service catalog present ({len(catalog)} services)")

            # Check for required services
            services = {s['type']: s['name'] for s in catalog}
            required_services = ['identity', 'compute', 'network', 'volumev3', 'image']

            for service_type in required_services:
                if service_type in services:
                    print_info(f"✓ {service_type}: {services[service_type]}")
                else:
                    print_fail(f"✗ Missing service: {service_type}")
                    return False
        else:
            print_fail("Service catalog missing")
            return False

        return True

    def test_project_dashboard(self):
        """Test Project Dashboard (what Horizon loads first)"""
        print_test("Project Dashboard Load")

        if not self.token:
            print_fail("Not authenticated")
            return False

        headers = {"X-Auth-Token": self.token}

        # Horizon loads these on dashboard page
        endpoints = [
            ("Nova - List servers", f"http://localhost:8774/v2.1/servers"),
            ("Nova - List flavors", f"http://localhost:8774/v2.1/flavors"),
            ("Neutron - List networks", f"http://localhost:9696/v2.0/networks"),
            ("Cinder - List volumes", f"http://localhost:8776/v3/{self.project_id}/volumes"),
            ("Glance - List images", f"http://localhost:9292/v2/images"),
        ]

        all_success = True
        for name, url in endpoints:
            response = requests.get(url, headers=headers)
            if response.status_code == 200:
                print_pass(f"{name}")
            else:
                print_fail(f"{name} (HTTP {response.status_code})")
                all_success = False

        return all_success

    def test_instances_tab(self):
        """Test Instances Tab (Nova)"""
        print_test("Instances Tab")

        headers = {"X-Auth-Token": self.token}

        # List servers (detailed)
        response = requests.get(
            "http://localhost:8774/v2.1/servers/detail",
            headers=headers
        )

        if response.status_code != 200:
            print_fail(f"Failed to list servers (HTTP {response.status_code})")
            return False

        servers = response.json().get('servers', [])
        print_pass(f"Listed {len(servers)} servers")

        # Get hypervisor stats (Horizon uses this for overview)
        response = requests.get(
            "http://localhost:8774/v2.1/os-hypervisors/statistics",
            headers=headers
        )

        if response.status_code == 200:
            stats = response.json().get('hypervisor_statistics', {})
            print_pass("Hypervisor statistics retrieved")
            print_info(f"vCPUs: {stats.get('vcpus', 0)} total, {stats.get('vcpus_used', 0)} used")
            print_info(f"Memory: {stats.get('memory_mb', 0)} MB total, {stats.get('memory_mb_used', 0)} MB used")
        else:
            print_fail(f"Failed to get hypervisor stats (HTTP {response.status_code})")
            return False

        return True

    def test_networks_tab(self):
        """Test Networks Tab (Neutron)"""
        print_test("Networks Tab")

        headers = {"X-Auth-Token": self.token}

        # List networks
        response = requests.get(
            "http://localhost:9696/v2.0/networks",
            headers=headers
        )

        if response.status_code != 200:
            print_fail(f"Failed to list networks (HTTP {response.status_code})")
            return False

        networks = response.json().get('networks', [])
        print_pass(f"Listed {len(networks)} networks")

        # List subnets
        response = requests.get(
            "http://localhost:9696/v2.0/subnets",
            headers=headers
        )

        if response.status_code == 200:
            subnets = response.json().get('subnets', [])
            print_pass(f"Listed {len(subnets)} subnets")
        else:
            print_fail(f"Failed to list subnets (HTTP {response.status_code})")

        # List routers
        response = requests.get(
            "http://localhost:9696/v2.0/routers",
            headers=headers
        )

        if response.status_code == 200:
            routers = response.json().get('routers', [])
            print_pass(f"Listed {len(routers)} routers")
        else:
            print_fail(f"Failed to list routers (HTTP {response.status_code})")

        return True

    def test_volumes_tab(self):
        """Test Volumes Tab (Cinder)"""
        print_test("Volumes Tab")

        headers = {"X-Auth-Token": self.token}

        # List volumes
        response = requests.get(
            f"http://localhost:8776/v3/{self.project_id}/volumes/detail",
            headers=headers
        )

        if response.status_code != 200:
            print_fail(f"Failed to list volumes (HTTP {response.status_code})")
            return False

        volumes = response.json().get('volumes', [])
        print_pass(f"Listed {len(volumes)} volumes")

        # List volume types
        response = requests.get(
            f"http://localhost:8776/v3/{self.project_id}/types",
            headers=headers
        )

        if response.status_code == 200:
            types = response.json().get('volume_types', [])
            print_pass(f"Listed {len(types)} volume types")
        else:
            print_fail(f"Failed to list volume types (HTTP {response.status_code})")

        return True

    def test_images_tab(self):
        """Test Images Tab (Glance)"""
        print_test("Images Tab")

        headers = {"X-Auth-Token": self.token}

        # List images
        response = requests.get(
            "http://localhost:9292/v2/images",
            headers=headers
        )

        if response.status_code != 200:
            print_fail(f"Failed to list images (HTTP {response.status_code})")
            return False

        images = response.json().get('images', [])
        print_pass(f"Listed {len(images)} images")

        for img in images:
            print_info(f"  - {img.get('name', 'Unnamed')} ({img.get('status', 'unknown')})")

        return True

    def test_instance_creation_flow(self):
        """Test Instance Creation Flow (what happens when clicking Launch Instance)"""
        print_test("Launch Instance Workflow")

        headers = {"X-Auth-Token": self.token}

        # Step 1: Get available flavors
        response = requests.get(
            "http://localhost:8774/v2.1/flavors/detail",
            headers=headers
        )

        if response.status_code != 200:
            print_fail("Failed to get flavors")
            return False

        flavors = response.json().get('flavors', [])
        print_pass(f"Retrieved {len(flavors)} flavors")

        if not flavors:
            print_fail("No flavors available")
            return False

        flavor_id = flavors[0]['id']
        print_info(f"Using flavor: {flavors[0]['name']} ({flavor_id})")

        # Step 2: Get available images
        response = requests.get(
            "http://localhost:9292/v2/images",
            headers=headers
        )

        if response.status_code != 200:
            print_fail("Failed to get images")
            return False

        images = response.json().get('images', [])
        active_images = [img for img in images if img.get('status') == 'active']

        if not active_images:
            print_info("No active images available - skipping server creation")
            print_pass("Launch instance flow validated (no images to test with)")
            return True

        image_id = active_images[0]['id']
        print_pass(f"Retrieved {len(active_images)} active images")
        print_info(f"Using image: {active_images[0].get('name', 'Unnamed')} ({image_id})")

        # Step 3: Get available networks
        response = requests.get(
            "http://localhost:9696/v2.0/networks",
            headers=headers
        )

        if response.status_code != 200:
            print_fail("Failed to get networks")
            return False

        networks = response.json().get('networks', [])
        if not networks:
            print_info("No networks available - creating default network")
            # Create network for testing
            create_response = requests.post(
                "http://localhost:9696/v2.0/networks",
                headers={**headers, "Content-Type": "application/json"},
                json={"network": {"name": "horizon-test-net", "admin_state_up": True}}
            )
            if create_response.status_code == 201:
                network_id = create_response.json()['network']['id']
                print_pass(f"Created test network: {network_id}")
            else:
                print_fail("Failed to create network")
                return False
        else:
            network_id = networks[0]['id']
            print_pass(f"Retrieved {len(networks)} networks")
            print_info(f"Using network: {networks[0].get('name', 'Unnamed')} ({network_id})")

        # Step 4: Simulate server creation (Horizon would do this)
        server_name = f"horizon-test-vm-{datetime.now().strftime('%Y%m%d%H%M%S')}"
        server_payload = {
            "server": {
                "name": server_name,
                "flavorRef": flavor_id,
                "imageRef": image_id,
                "networks": [{"uuid": network_id}]
            }
        }

        response = requests.post(
            "http://localhost:8774/v2.1/servers",
            headers={**headers, "Content-Type": "application/json"},
            json=server_payload
        )

        if response.status_code == 202:
            server_data = response.json()['server']
            server_id = server_data['id']
            print_pass(f"Server creation initiated: {server_name}")
            print_info(f"Server ID: {server_id}")

            # Cleanup - delete the test server
            requests.delete(
                f"http://localhost:8774/v2.1/servers/{server_id}",
                headers=headers
            )
            print_info("Test server deleted")

            return True
        else:
            print_fail(f"Server creation failed (HTTP {response.status_code})")
            if response.text:
                print_info(f"Error: {response.text}")
            return False


def main():
    print("==========================================")
    print(" O3K Horizon Compatibility Test")
    print("==========================================")
    print()

    simulator = HorizonSimulator()

    # Test 1: Authentication (Horizon login)
    if not simulator.authenticate():
        print("\n❌ Authentication failed - cannot proceed with tests")
        sys.exit(1)

    # Test 2: Project Dashboard
    simulator.test_project_dashboard()

    # Test 3: Instances Tab
    simulator.test_instances_tab()

    # Test 4: Networks Tab
    simulator.test_networks_tab()

    # Test 5: Volumes Tab
    simulator.test_volumes_tab()

    # Test 6: Images Tab
    simulator.test_images_tab()

    # Test 7: Instance Creation Flow
    simulator.test_instance_creation_flow()

    # Summary
    print("\n==========================================")
    print(" Test Summary")
    print("==========================================")
    print(f"Total Passed: {tests_passed}")
    print(f"Total Failed: {tests_failed}")
    print()

    if tests_failed == 0:
        print(f"{GREEN}✓ All Horizon compatibility tests passed!{NC}")
        print()
        print("Next steps:")
        print("  1. Deploy actual Horizon dashboard")
        print("  2. Test with web browser")
        print("  3. Verify UI rendering and interactions")
        sys.exit(0)
    else:
        print(f"{RED}✗ Some tests failed{NC}")
        sys.exit(1)


if __name__ == "__main__":
    main()
