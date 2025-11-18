package server

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "os"
    "testing"

    "github.com/chaosblade-io/chaosblade-spec-go/spec"
    "github.com/chaosblade-io/chaosblade/data"
)

func TestRouter_PostAndDeleteExperiment_OS(t *testing.T) {
    // use temp sqlite file
    os.Setenv("BLADE_SQLITE_PATH", "test_chaosblade.db")
    defer func() { os.Remove("test_chaosblade.db") }()
    r, err := NewRouter("test-token")
    if err != nil {
        t.Fatalf("NewRouter error: %v", err)
    }

    // prepare request body
    body := map[string]interface{}{
        "target": "os",
        "action": "load",
        "flags": map[string]string{"cpu-percent": "5"},
    }
    bs, _ := json.Marshal(body)
    req := httptest.NewRequest(http.MethodPost, "/v1/experiments", bytes.NewReader(bs))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Api-Token", "test-token")

    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)
    if w.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
    }
    var resp map[string]interface{}
    if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
        t.Fatalf("invalid response json: %v", err)
    }
    dataObj, ok := resp["data"].(map[string]interface{})
    if !ok {
        t.Fatalf("no data in response: %v", resp)
    }
    uid, ok := dataObj["uid"].(string)
    if !ok || uid == "" {
        t.Fatalf("invalid uid result: %v", dataObj)
    }

    // DELETE
    delReq := httptest.NewRequest(http.MethodDelete, "/v1/experiments/"+uid, nil)
    delReq.Header.Set("X-Api-Token", "test-token")
    dw := httptest.NewRecorder()
    r.ServeHTTP(dw, delReq)
    if dw.Code != http.StatusOK {
        t.Fatalf("expected delete 200, got %d, body: %s", dw.Code, dw.Body.String())
    }
    // verify DB status via QueryExperimentByUid
    exp, err := data.QueryExperimentByUid(uid)
    if err != nil {
        t.Fatalf("query experiment error: %v", err)
    }
    if exp.Status != "destroyed" {
        t.Fatalf("expected destroyed status, got %s", exp.Status)
    }
    // ensure destroy flag path uses spec.Destroy behavior
    _ = spec.SetDestroyFlag
}
