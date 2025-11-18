package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"

	"github.com/chaosblade-io/chaosblade/pkg/server/middleware"
	"github.com/chaosblade-io/chaosblade/pkg/service/experiment"
)

// Server wraps gin.Engine to expose REST endpoints.
type Server struct {
	engine  *gin.Engine
	service *experiment.Service
}

// NewServer builds a gin server with middleware and routes.
func NewServer(svc *experiment.Service, authToken string) *Server {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.AuditMiddleware())
	router.Use(middleware.NewIdempotencyMiddleware(10 * time.Minute).Handler())
	router.Use(middleware.AuthMiddleware(authToken))

	srv := &Server{engine: router, service: svc}
	srv.registerRoutes()
	return srv
}

// Engine exposes the underlying gin engine for bootstrapping.
func (s *Server) Engine() *gin.Engine {
	return s.engine
}

func (s *Server) registerRoutes() {
	api := s.engine.Group("/api/v1")
	api.GET("/experiments/:uid", s.handleGetExperiment)
	api.POST("/experiments", s.handleCreateExperiment)
	api.DELETE("/experiments/:uid", s.handleDestroyExperiment)
	api.GET("/openapi", func(c *gin.Context) {
		c.File("docs/openapi.yaml")
	})
}

func (s *Server) handleCreateExperiment(c *gin.Context) {
	var request experiment.CreateExperimentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	response, model, err := s.service.Create(c.Request.Context(), request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"response": response, "record": model})
}

func (s *Server) handleDestroyExperiment(c *gin.Context) {
	uid := c.Param("uid")
	var request experiment.DestroyExperimentRequest
	if c.Request.Body != nil {
		_ = c.ShouldBindJSON(&request)
	}
	request.UID = uid
	response, err := s.service.Destroy(c.Request.Context(), request)
	if err != nil {
		if resp, ok := err.(*spec.Response); ok {
			c.JSON(http.StatusBadRequest, resp)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, response)
}

func (s *Server) handleGetExperiment(c *gin.Context) {
	uid := c.Param("uid")
	model, err := s.service.Query(uid)
	if err != nil {
		if resp, ok := err.(*spec.Response); ok {
			c.JSON(http.StatusBadRequest, resp)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, model)
}
