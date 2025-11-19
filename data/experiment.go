/*
 * Copyright 2025 The ChaosBlade Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package data

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/chaosblade-io/chaosblade-spec-go/log"
	"github.com/chaosblade-io/chaosblade-spec-go/spec"
)

type ExperimentModel struct {
	ID         uint   `gorm:"primaryKey"`
	Uid        string `gorm:"column:uid;uniqueIndex:exp_uid_uidx;size:32"`
	Command    string `gorm:"column:command;index:exp_command_idx"`
	SubCommand string `gorm:"column:sub_command"`
	Flag       string `gorm:"column:flag"`
	Status     string `gorm:"column:status;index:exp_status_idx"`
	Error      string `gorm:"column:error"`
	CreateTime string `gorm:"column:create_time"`
	UpdateTime string `gorm:"column:update_time"`
}

func (ExperimentModel) TableName() string {
	return "experiment"
}

type ExperimentSource interface {
	// CheckAndInitExperimentTable, if experiment table not exists, then init it
	CheckAndInitExperimentTable()

	// ExperimentTableExists return true if experiment exists
	ExperimentTableExists() (bool, error)

	// InitExperimentTable for first executed
	InitExperimentTable() error

	// InsertExperimentModel for creating chaos experiment
	InsertExperimentModel(model *ExperimentModel) error

	// UpdateExperimentModelByUid
	UpdateExperimentModelByUid(uid, status, errMsg string) error

	// QueryExperimentModelByUid
	QueryExperimentModelByUid(uid string) (*ExperimentModel, error)

	// QueryExperimentModels
	QueryExperimentModels(target, action, flag, status, limit string, asc bool) ([]*ExperimentModel, error)

	// QueryExperimentModelsByCommand
	// flags value contains necessary parameters generally
	QueryExperimentModelsByCommand(command, subCommand string, flags map[string]string) ([]*ExperimentModel, error)

	// DeleteExperimentModelByUid
	DeleteExperimentModelByUid(uid string) error
}

func (s *Source) CheckAndInitExperimentTable() {
	if err := s.InitExperimentTable(); err != nil {
		log.Fatalf(context.Background(), "%s", err.Error())
	}
}

func (s *Source) ExperimentTableExists() (bool, error) {
	return s.DB.Migrator().HasTable(&ExperimentModel{}), nil
}

func (s *Source) InitExperimentTable() error {
	return s.DB.AutoMigrate(&ExperimentModel{})
}

func (s *Source) InsertExperimentModel(model *ExperimentModel) error {
	if model.CreateTime == "" {
		model.CreateTime = time.Now().Format(time.RFC3339Nano)
	}
	if model.UpdateTime == "" {
		model.UpdateTime = model.CreateTime
	}
	return s.DB.Create(model).Error
}

func (s *Source) UpdateExperimentModelByUid(uid, status, errMsg string) error {
	return s.DB.Model(&ExperimentModel{}).
		Where("uid = ?", uid).
		Updates(map[string]interface{}{
			"status":      status,
			"error":       errMsg,
			"update_time": time.Now().Format(time.RFC3339Nano),
		}).Error
}

func (s *Source) QueryExperimentModelByUid(uid string) (*ExperimentModel, error) {
	var model ExperimentModel
	err := s.DB.Where("uid = ?", uid).First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &model, nil
}

func (s *Source) QueryExperimentModels(target, action, flag, status, limit string, asc bool) ([]*ExperimentModel, error) {
	db := s.DB.Model(&ExperimentModel{})
	if target != "" {
		db = db.Where("command = ?", target)
	}
	if action != "" {
		db = db.Where("sub_command = ?", action)
	}
	if flag != "" {
		db = db.Where("flag LIKE ?", "%"+flag+"%")
	}
	if status != "" {
		db = db.Where("status = ?", UpperFirst(status))
	}
	if asc {
		db = db.Order("id asc")
	} else {
		db = db.Order("id desc")
	}
	if limit != "" {
		values := strings.Split(limit, ",")
		switch len(values) {
		case 1:
			if count, err := strconv.Atoi(values[0]); err == nil {
				db = db.Limit(count)
			}
		default:
			if offset, err := strconv.Atoi(values[0]); err == nil {
				db = db.Offset(offset)
			}
			if count, err := strconv.Atoi(values[1]); err == nil {
				db = db.Limit(count)
			}
		}
	}
	var models []*ExperimentModel
	if err := db.Find(&models).Error; err != nil {
		return nil, err
	}
	return models, nil
}

func (s *Source) QueryExperimentModelsByCommand(command, subCommand string, flags map[string]string) ([]*ExperimentModel, error) {
	models := make([]*ExperimentModel, 0)
	experimentModels, err := s.QueryExperimentModels(command, subCommand, "", "", "", true)
	if err != nil {
		return models, err
	}
	if flags == nil || len(flags) == 0 {
		return experimentModels, nil
	}
	for _, experimentModel := range experimentModels {
		recordModel := spec.ConvertCommandsToExpModel(subCommand, command, experimentModel.Flag)
		recordFlags := recordModel.ActionFlags
		isMatched := true
		for k, v := range flags {
			if v == "" {
				continue
			}
			if recordFlags[k] != v {
				isMatched = false
				break
			}
		}
		if isMatched {
			models = append(models, experimentModel)
		}
	}
	return models, nil
}

func (s *Source) DeleteExperimentModelByUid(uid string) error {
	return s.DB.Where("uid = ?", uid).Delete(&ExperimentModel{}).Error
}
