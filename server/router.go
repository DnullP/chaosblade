package server

import (
	"net/http"
	"os"

	"github.com/chaosblade-io/chaosblade-spec-go/spec"
	"github.com/chaosblade-io/chaosblade/data"
	execos "github.com/chaosblade-io/chaosblade/exec/os"
	"github.com/gin-gonic/gin"
)

// NewRouter builds the Gin engine for the PoC server; token parameter can be empty to use env/default.
func NewRouter(token string) (*gin.Engine, error) {
	if err := data.InitDB(); err != nil {
		return nil, err
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	if token == "" {
		token = os.Getenv("BLADE_API_TOKEN")
		if token == "" {
			token = "secret-token"
		}
	}

	// auth middleware
	r.Use(func(c *gin.Context) {
		t := c.GetHeader("X-Api-Token")
		if t == "" || t != token {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
			return
		}
		c.Next()
	})

	// POST /v1/experiments
	r.POST("/v1/experiments", func(c *gin.Context) {
		var req struct {
			Target   string            `json:"target" binding:"required"`
			Action   string            `json:"action" binding:"required"`
			Scope    string            `json:"scope"`
			Flags    map[string]string `json:"flags"`
			Async    bool              `json:"async"`
			Timeout  int               `json:"timeout"`
			Callback string            `json:"callback"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
			return
		}

		model := &spec.ExpModel{Target: req.Target, ActionName: req.Action, ActionFlags: req.Flags}
		exp, err := data.CreateExperiment(model, req.Timeout, req.Callback)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
			return
		}

		if req.Target == "os" {
			executorIf := execos.NewExecutor()
			if real, ok := executorIf.(*execos.Executor); ok {
				resp := real.ExecInProcess(exp.Uid, c.Request.Context(), model)
				if !resp.Success {
					data.UpdateExperimentStatus(exp.Uid, "failed", resp.Err)
					c.JSON(http.StatusOK, gin.H{"code": resp.Code, "message": resp.Err, "data": gin.H{"uid": exp.Uid}})
					return
				}
				data.UpdateExperimentStatus(exp.Uid, "running", "")
				c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": gin.H{"uid": exp.Uid}})
				return
			}
			data.UpdateExperimentStatus(exp.Uid, "failed", "executor type mismatch")
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "executor type mismatch"})
			return
		}

		data.UpdateExperimentStatus(exp.Uid, "failed", "target not supported in PoC")
		c.JSON(http.StatusNotImplemented, gin.H{"code": 501, "message": "target not supported in PoC"})
	})

	// DELETE /v1/experiments/:uid
	r.DELETE("/v1/experiments/:uid", func(c *gin.Context) {
		uid := c.Param("uid")
		exp, err := data.QueryExperimentByUid(uid)
		if err != nil || exp == nil {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "not found"})
			return
		}
		if exp.Target == "os" {
			executorIf := execos.NewExecutor()
			model := &spec.ExpModel{Target: exp.Target, ActionName: exp.Action}
			if real, ok := executorIf.(*execos.Executor); ok {
				resp := real.ExecInProcess(uid, c.Request.Context(), model)
				if !resp.Success {
					c.JSON(http.StatusInternalServerError, gin.H{"code": resp.Code, "message": resp.Err})
					return
				}
				data.UpdateExperimentStatus(uid, "destroyed", "")
				c.JSON(http.StatusOK, gin.H{"code": 0, "message": "destroyed"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "executor type mismatch"})
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{"code": 501, "message": "target not supported in PoC"})
	})

	return r, nil
}
