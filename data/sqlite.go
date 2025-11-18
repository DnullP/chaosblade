package data

import (
    "encoding/json"
    "os"
    "time"

    "github.com/glebarez/sqlite"
    "gorm.io/gorm"
)

var db *gorm.DB

type Experiment struct {
    ID        uint           `gorm:"primaryKey" json:"-"`
    Uid       string         `gorm:"uniqueIndex;size:64" json:"uid"`
    Target    string         `json:"target"`
    Action    string         `json:"action"`
    Flags     string         `gorm:"type:TEXT" json:"flags"`
    Status    string         `json:"status"`
    ErrMsg    string         `json:"err_msg"`
    Timeout   int            `json:"timeout"`
    Callback  string         `json:"callback"`
    CreatedAt time.Time      `json:"created_at"`
    UpdatedAt time.Time      `json:"updated_at"`
    StartedAt *time.Time     `json:"started_at"`
    FinishedAt *time.Time    `json:"finished_at"`
}

func InitDB() error {
    path := os.Getenv("BLADE_SQLITE_PATH")
    if path == "" {
        path = "./chaosblade.db"
    }
    d, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
    if err != nil {
        return err
    }
    db = d
    return db.AutoMigrate(&Experiment{})
}

func CreateExperiment(model interface{}, timeout int, callback string) (*Experiment, error) {
    // model expected to be *spec.ExpModel, but avoid importing spec to reduce coupling here
    // Instead marshal model to JSON for flags
    bs, _ := json.Marshal(model)
    uid := time.Now().Format("20060102150405.000000")
    exp := &Experiment{
        Uid: uid,
        Target: extractStringField(model, "Target"),
        Action: extractStringField(model, "ActionName"),
        Flags: string(bs),
        Status: "created",
        Timeout: timeout,
        Callback: callback,
    }
    if err := db.Create(exp).Error; err != nil {
        return nil, err
    }
    return exp, nil
}

func UpdateExperimentStatus(uid, status, errMsg string) error {
    return db.Model(&Experiment{}).Where("uid = ?", uid).Updates(map[string]interface{}{"status": status, "err_msg": errMsg}).Error
}

func QueryExperimentByUid(uid string) (*Experiment, error) {
    var exp Experiment
    if err := db.Where("uid = ?", uid).First(&exp).Error; err != nil {
        return nil, err
    }
    return &exp, nil
}

// helper extract simple fields via type assertion
func extractStringField(v interface{}, name string) string {
    // naive implementation: try to marshal and read map
    mbs, err := json.Marshal(v)
    if err != nil {
        return ""
    }
    var mm map[string]interface{}
    if err := json.Unmarshal(mbs, &mm); err != nil {
        return ""
    }
    if val, ok := mm[name]; ok {
        if s, ok := val.(string); ok {
            return s
        }
    }
    return ""
}
