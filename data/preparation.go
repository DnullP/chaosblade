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
)

type PreparationRecord struct {
	ID          uint   `gorm:"primaryKey"`
	Uid         string `gorm:"column:uid;uniqueIndex:pre_uid_uidx;size:32"`
	ProgramType string `gorm:"column:program_type;index:pre_type_process_idx,priority:1"`
	Process     string `gorm:"column:process;index:pre_type_process_idx,priority:2"`
	Port        string `gorm:"column:port"`
	Pid         string `gorm:"column:pid"`
	Status      string `gorm:"column:status;index:pre_status_idx"`
	Error       string `gorm:"column:error"`
	CreateTime  string `gorm:"column:create_time"`
	UpdateTime  string `gorm:"column:update_time"`
}

func (PreparationRecord) TableName() string {
	return "preparation"
}

type PreparationSource interface {
	// CheckAndInitPreTable
	CheckAndInitPreTable()

	// InitPreparationTable when first executed
	InitPreparationTable() error

	// PreparationTableExists return true if preparation exists, otherwise return false or error if execute sql exception
	PreparationTableExists() (bool, error)

	// InsertPreparationRecord
	InsertPreparationRecord(record *PreparationRecord) error

	// QueryPreparationByUid
	QueryPreparationByUid(uid string) (*PreparationRecord, error)

	// QueryRunningPreByTypeAndProcess
	QueryRunningPreByTypeAndProcess(programType string, processName, processId string) (*PreparationRecord, error)

	// UpdatePreparationRecordByUid
	UpdatePreparationRecordByUid(uid, status, errMsg string) error

	// UpdatePreparationPortByUid
	UpdatePreparationPortByUid(uid, port string) error

	// UpdatePreparationPidByUid
	UpdatePreparationPidByUid(uid, pid string) error

	// QueryPreparationRecords
	QueryPreparationRecords(target, status, action, flag, limit string, asc bool) ([]*PreparationRecord, error)
}

// UserVersion PRAGMA [database.]user_version
const UserVersion = 1

// addPidColumn sql
const addPidColumn = `ALTER TABLE preparation ADD COLUMN pid VARCHAR DEFAULT ""`

func (s *Source) CheckAndInitPreTable() {
	// check user_version
	version, err := s.GetUserVersion()
	ctx := context.Background()
	if err != nil {
		log.Fatalf(ctx, "%s", err.Error())
	}
	// return directly if equal the current UserVersion
	if version == UserVersion {
		// still ensure indexes/columns match the struct definition
		_ = s.InitPreparationTable()
		return
	}
	// check the table exists or not
	exists, err := s.PreparationTableExists()
	if err != nil {
		log.Fatalf(ctx, "%s", err.Error())
	}
	if exists {
		// check if pid column exists before adding it
		pidColumnExists, err := s.ColumnExists("preparation", "pid")
		if err != nil {
			log.Fatalf(ctx, "%s", err.Error())
		}
		if !pidColumnExists {
			// execute alter sql if column doesn't exist
			err := s.AlterPreparationTable(addPidColumn)
			if err != nil {
				log.Fatalf(ctx, "%s", err.Error())
			}
		}
	} else {
		// execute create table
		err = s.InitPreparationTable()
		if err != nil {
			log.Fatalf(ctx, "%s", err.Error())
		}
	}
	// update userVersion to new
	err = s.UpdateUserVersion(UserVersion)
	if err != nil {
		log.Fatalf(ctx, "%s", err.Error())
	}
}

func (s *Source) InitPreparationTable() error {
	return s.DB.AutoMigrate(&PreparationRecord{})
}

func (s *Source) AlterPreparationTable(alterSql string) error {
	return s.DB.Exec(alterSql).Error
}

func (s *Source) PreparationTableExists() (bool, error) {
	return s.DB.Migrator().HasTable(&PreparationRecord{}), nil
}

func (s *Source) InsertPreparationRecord(record *PreparationRecord) error {
	now := time.Now().Format(time.RFC3339Nano)
	if record.CreateTime == "" {
		record.CreateTime = now
	}
	if record.UpdateTime == "" {
		record.UpdateTime = record.CreateTime
	}
	return s.DB.Create(record).Error
}

func (s *Source) QueryPreparationByUid(uid string) (*PreparationRecord, error) {
	var record PreparationRecord
	err := s.DB.Where("uid = ?", uid).First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

// QueryRunningPreByTypeAndProcess returns the first record matching the process id or process name
func (s *Source) QueryRunningPreByTypeAndProcess(programType string, processName, processId string) (*PreparationRecord, error) {
	db := s.DB.Where("program_type = ? AND status = ?", programType, "Running")
	if processId != "" {
		db = db.Where("pid = ?", processId)
	}
	if processName != "" {
		db = db.Where("process = ?", processName)
	}
	var record PreparationRecord
	err := db.First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (s *Source) UpdatePreparationRecordByUid(uid, status, errMsg string) error {
	return s.DB.Model(&PreparationRecord{}).
		Where("uid = ?", uid).
		Updates(map[string]interface{}{
			"status":      status,
			"error":       errMsg,
			"update_time": time.Now().Format(time.RFC3339Nano),
		}).Error
}

func (s *Source) UpdatePreparationPortByUid(uid, port string) error {
	return s.DB.Model(&PreparationRecord{}).
		Where("uid = ?", uid).
		Updates(map[string]interface{}{
			"port":        port,
			"update_time": time.Now().Format(time.RFC3339Nano),
		}).Error
}

func (s *Source) UpdatePreparationPidByUid(uid, pid string) error {
	return s.DB.Model(&PreparationRecord{}).
		Where("uid = ?", uid).
		Updates(map[string]interface{}{
			"pid":         pid,
			"update_time": time.Now().Format(time.RFC3339Nano),
		}).Error
}

func (s *Source) QueryPreparationRecords(target, status, action, flag, limit string, asc bool) ([]*PreparationRecord, error) {
	db := s.DB.Model(&PreparationRecord{})
	if target != "" {
		db = db.Where("program_type = ?", target)
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
	var records []*PreparationRecord
	if err := db.Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}
