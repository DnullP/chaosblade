package gin

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type HandlerFunc func(*Context)

type Engine struct {
	RouterGroup
	routes []*route
}

type RouterGroup struct {
	basePath string
	handlers []HandlerFunc
	engine   *Engine
}

type route struct {
	method   string
	pattern  string
	handlers []HandlerFunc
}

type H map[string]interface{}

func New() *Engine {
	engine := &Engine{}
	engine.RouterGroup = RouterGroup{engine: engine}
	return engine
}

func Default() *Engine { return New() }

func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
	for _, rt := range e.routes {
		if !strings.EqualFold(rt.method, r.Method) {
			continue
		}
		params, ok := match(rt.pattern, r.URL.Path)
		if !ok {
			continue
		}
		ctx := &Context{
			Request: r,
			Writer:  rw,
			params:  params,
		}
		ctx.handlers = append(ctx.handlers, e.handlers...)
		ctx.handlers = append(ctx.handlers, rt.handlers...)
		ctx.Next()
		return
	}
	http.NotFound(rw, r)
}

// Run starts the HTTP server.
func (e *Engine) Run(addr string) error {
	return http.ListenAndServe(addr, e)
}

// Use appends middleware to the group.
func (g *RouterGroup) Use(middleware ...HandlerFunc) {
	g.handlers = append(g.handlers, middleware...)
}

// Group builds a new router group with prefix.
func (g *RouterGroup) Group(relativePath string) *RouterGroup {
	return &RouterGroup{basePath: joinPaths(g.basePath, relativePath), handlers: append([]HandlerFunc{}, g.handlers...), engine: g.engine}
}

// Handle registers a new route.
func (g *RouterGroup) Handle(method, relativePath string, handlers ...HandlerFunc) {
	fullPath := joinPaths(g.basePath, relativePath)
	g.engine.routes = append(g.engine.routes, &route{method: method, pattern: fullPath, handlers: append(append([]HandlerFunc{}, g.handlers...), handlers...)})
}

func (g *RouterGroup) GET(relativePath string, handlers ...HandlerFunc) {
	g.Handle(http.MethodGet, relativePath, handlers...)
}
func (g *RouterGroup) POST(relativePath string, handlers ...HandlerFunc) {
	g.Handle(http.MethodPost, relativePath, handlers...)
}
func (g *RouterGroup) DELETE(relativePath string, handlers ...HandlerFunc) {
	g.Handle(http.MethodDelete, relativePath, handlers...)
}

// File serves a static file.
func (c *Context) File(file string) {
	http.ServeFile(c.Writer, c.Request, filepath.Clean(file))
}

// Context mirrors the pieces of gin.Context used in this repository.
type Context struct {
	Request  *http.Request
	Writer   *responseWriter
	params   map[string]string
	handlers []HandlerFunc
	index    int
	aborted  bool
}

// Next executes the remaining handlers.
func (c *Context) Next() {
	for c.index < len(c.handlers) {
		handler := c.handlers[c.index]
		c.index++
		handler(c)
		if c.aborted {
			return
		}
	}
}

// ShouldBindJSON decodes the request body into the provided struct.
func (c *Context) ShouldBindJSON(obj interface{}) error {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(obj)
}

// JSON writes a JSON response with status code.
func (c *Context) JSON(code int, obj interface{}) {
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(code)
	_ = json.NewEncoder(c.Writer).Encode(obj)
}

// Param returns a path parameter.
func (c *Context) Param(key string) string { return c.params[key] }

// GetHeader gets a request header value.
func (c *Context) GetHeader(key string) string { return c.Request.Header.Get(key) }

// AbortWithStatusJSON stops the handler chain and writes an error response.
func (c *Context) AbortWithStatusJSON(code int, obj interface{}) {
	c.aborted = true
	c.JSON(code, obj)
}

// ResponseWriter captures the status code for audit logging.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriter) Status() int { return w.status }

func joinPaths(base, relative string) string {
	if relative == "" {
		return base
	}
	if base == "" || base == "/" {
		return relative
	}
	return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(relative, "/")
}

func match(pattern, path string) (map[string]string, bool) {
	params := make(map[string]string)
	pSegs := strings.Split(strings.Trim(pattern, "/"), "/")
	pathSegs := strings.Split(strings.Trim(path, "/"), "/")
	if len(pSegs) != len(pathSegs) {
		return nil, false
	}
	for i := range pSegs {
		if strings.HasPrefix(pSegs[i], ":") {
			params[strings.TrimPrefix(pSegs[i], ":")] = pathSegs[i]
			continue
		}
		if pSegs[i] != pathSegs[i] {
			return nil, false
		}
	}
	return params, true
}

// Recovery is a lightweight panic recovery middleware.
func Recovery() HandlerFunc {
	return func(c *Context) {
		defer func() {
			if r := recover(); r != nil {
				http.Error(c.Writer, "internal server error", http.StatusInternalServerError)
				c.aborted = true
			}
		}()
		c.Next()
	}
}

// New returns an empty H map for JSON helpers.
func HFromPairs(pairs ...interface{}) H {
	h := H{}
	for i := 0; i < len(pairs)-1; i += 2 {
		key, _ := pairs[i].(string)
		h[key] = pairs[i+1]
	}
	return h
}

// SetTrustedProxies is kept for compatibility; no-op in this lightweight shim.
func (e *Engine) SetTrustedProxies(_ []string) error { return nil }

// Logger is a no-op middleware placeholder.
func Logger() HandlerFunc {
	return func(*Context) {}
}

// AbortWithStatus writes status code.
func (c *Context) AbortWithStatus(code int) { c.AbortWithStatusJSON(code, H{}) }

// Abort sets aborted flag without writing output.
func (c *Context) Abort() { c.aborted = true }

// MustGet returns values stored in context Keys map. Not used in shim.
func (c *Context) MustGet(key string) interface{} { return nil }

// Redirect provides simple redirect helper.
func (c *Context) Redirect(code int, location string) {
	http.Redirect(c.Writer, c.Request, location, code)
}

// Status returns the current status code.
func (c *Context) Status(code int) { c.Writer.WriteHeader(code) }

func init() {
	// ensure docs directory exists when serving files
	_ = os.MkdirAll("docs", 0o755)
}
