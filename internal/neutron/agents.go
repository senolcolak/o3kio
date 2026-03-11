package neutron

import (
	"net/http"
	"time"

	"github.com/cobaltcore-dev/o3k/internal/database"
	"github.com/gin-gonic/gin"
)

// ListAgents returns all network agents
func (svc *Service) ListAgents(c *gin.Context) {
	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT id, agent_type, "binary", host, description, admin_state_up, alive, started_at, created_at, configurations
		FROM neutron_agents
		ORDER BY created_at DESC
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query agents"})
		return
	}
	defer rows.Close()

	agents := []map[string]interface{}{}
	for rows.Next() {
		var id, agentType, binary, host string
		var description *string
		var adminStateUp, alive bool
		var startedAt, createdAt time.Time
		var configurations map[string]interface{}

		if err := rows.Scan(&id, &agentType, &binary, &host, &description, &adminStateUp, &alive, &startedAt, &createdAt, &configurations); err != nil {
			continue
		}

		agent := map[string]interface{}{
			"id":              id,
			"agent_type":      agentType,
			"binary":          binary,
			"host":            host,
			"admin_state_up":  adminStateUp,
			"alive":           alive,
			"started_at":      startedAt.Format(time.RFC3339),
			"created_at":      createdAt.Format(time.RFC3339),
			"configurations":  configurations,
		}

		if description != nil {
			agent["description"] = *description
		}

		agents = append(agents, agent)
	}

	c.JSON(http.StatusOK, gin.H{"agents": agents})
}

// GetAgent returns a specific agent by ID
func (svc *Service) GetAgent(c *gin.Context) {
	agentID := c.Param("id")

	var id, agentType, binary, host string
	var description *string
	var adminStateUp, alive bool
	var startedAt, createdAt time.Time
	var configurations map[string]interface{}

	err := database.DB.QueryRow(c.Request.Context(), `
		SELECT id, agent_type, "binary", host, description, admin_state_up, alive, started_at, created_at, configurations
		FROM neutron_agents
		WHERE id = $1
	`, agentID).Scan(&id, &agentType, &binary, &host, &description, &adminStateUp, &alive, &startedAt, &createdAt, &configurations)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	agent := map[string]interface{}{
		"id":              id,
		"agent_type":      agentType,
		"binary":          binary,
		"host":            host,
		"admin_state_up":  adminStateUp,
		"alive":           alive,
		"started_at":      startedAt.Format(time.RFC3339),
		"created_at":      createdAt.Format(time.RFC3339),
		"configurations":  configurations,
	}

	if description != nil {
		agent["description"] = *description
	}

	c.JSON(http.StatusOK, gin.H{"agent": agent})
}

// ListL3AgentsOnRouter returns L3 agents hosting a specific router
func (svc *Service) ListL3AgentsOnRouter(c *gin.Context) {
	routerID := c.Param("id")

	// Verify router exists
	var exists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM routers WHERE id = $1)",
		routerID).Scan(&exists)
	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Router not found"})
		return
	}

	rows, err := database.DB.Query(c.Request.Context(), `
		SELECT a.id, a.agent_type, a."binary", a.host, a.description, a.admin_state_up, a.alive, a.started_at, a.created_at, a.configurations
		FROM neutron_agents a
		JOIN router_l3_agents r ON a.id = r.agent_id
		WHERE r.router_id = $1
		ORDER BY a.created_at DESC
	`, routerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query agents"})
		return
	}
	defer rows.Close()

	agents := []map[string]interface{}{}
	for rows.Next() {
		var id, agentType, binary, host string
		var description *string
		var adminStateUp, alive bool
		var startedAt, createdAt time.Time
		var configurations map[string]interface{}

		if err := rows.Scan(&id, &agentType, &binary, &host, &description, &adminStateUp, &alive, &startedAt, &createdAt, &configurations); err != nil {
			continue
		}

		agent := map[string]interface{}{
			"id":              id,
			"agent_type":      agentType,
			"binary":          binary,
			"host":            host,
			"admin_state_up":  adminStateUp,
			"alive":           alive,
			"started_at":      startedAt.Format(time.RFC3339),
			"created_at":      createdAt.Format(time.RFC3339),
			"configurations":  configurations,
		}

		if description != nil {
			agent["description"] = *description
		}

		agents = append(agents, agent)
	}

	c.JSON(http.StatusOK, gin.H{"agents": agents})
}

// AddL3AgentToRouter schedules a router to an L3 agent
func (svc *Service) AddL3AgentToRouter(c *gin.Context) {
	routerID := c.Param("id")

	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Verify router exists
	var routerExists bool
	err := database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM routers WHERE id = $1)",
		routerID).Scan(&routerExists)
	if err != nil || !routerExists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Router not found"})
		return
	}

	// Verify agent exists
	var agentExists bool
	err = database.DB.QueryRow(c.Request.Context(),
		"SELECT EXISTS(SELECT 1 FROM neutron_agents WHERE id = $1)",
		req.AgentID).Scan(&agentExists)
	if err != nil || !agentExists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	// Add association
	_, err = database.DB.Exec(c.Request.Context(), `
		INSERT INTO router_l3_agents (router_id, agent_id, created_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (router_id, agent_id) DO NOTHING
	`, routerID, req.AgentID, time.Now())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add agent to router"})
		return
	}

	c.Status(http.StatusCreated)
}
