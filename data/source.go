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
	"fmt"
	"os"
	"path"
	"sync"
	"unicode"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/chaosblade-io/chaosblade-spec-go/log"
	"github.com/chaosblade-io/chaosblade-spec-go/util"
)

const dataFile = "chaosblade.dat"

type SourceI interface {
	ExperimentSource
	PreparationSource
}

type Source struct {
	DB *gorm.DB
}

var (
	source SourceI
	once   = sync.Once{}
)

func GetSource() SourceI {
	once.Do(func() {
		src := &Source{
			DB: getConnection(),
		}
		src.init()
		source = src
	})
	return source
}

func (s *Source) init() {
	s.CheckAndInitExperimentTable()
	s.CheckAndInitPreTable()
}

// GetDataFilePath gets the data file path.
// Prioritizes reading from the CHAOSBLADE_DATAFILE_PATH environment variable.
// If CHAOSBLADE_DATAFILE_PATH is a directory, it is used as the directory for dataFile.
// If CHAOSBLADE_DATAFILE_PATH is a file, it is used as the file for dataFile.
// If CHAOSBLADE_DATAFILE_PATH is not specified, the original logic is used.
func GetDataFilePath() string {
	envPath := os.Getenv("CHAOSBLADE_DATAFILE_PATH")
	if envPath == "" {
		return path.Join(util.GetProgramPath(), dataFile)
	}

	fileInfo, err := os.Stat(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			if path.Ext(envPath) != "" {
				parentDir := path.Dir(envPath)
				if mkdirErr := os.MkdirAll(parentDir, 0o755); mkdirErr != nil {
					log.Warnf(context.Background(), "failed to create parent directory %s, using default path: %s", parentDir, mkdirErr.Error())
					return path.Join(util.GetProgramPath(), dataFile)
				}
				return envPath
			} else {
				if mkdirErr := os.MkdirAll(envPath, 0o755); mkdirErr != nil {
					log.Warnf(context.Background(), "failed to create directory %s, using default path: %s", envPath, mkdirErr.Error())
					return path.Join(util.GetProgramPath(), dataFile)
				}
				return path.Join(envPath, dataFile)
			}
		}
		log.Warnf(context.Background(), "failed to stat path %s, using default path: %s", envPath, err.Error())
		return path.Join(util.GetProgramPath(), dataFile)
	}

	if fileInfo.IsDir() {
		return path.Join(envPath, dataFile)
	} else {
		return envPath
	}
}

func getConnection() *gorm.DB {
	database, err := gorm.Open(sqlite.Open(GetDataFilePath()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		log.Fatalf(context.Background(), "open data file err, %s", err.Error())
	}
	return database
}

func (s *Source) Close() {
	if s.DB != nil {
		sqlDB, err := s.DB.DB()
		if err == nil {
			sqlDB.Close()
		}
	}
}

// GetUserVersion returns the user_version value
func (s *Source) GetUserVersion() (int, error) {
	var userVersion int
	if err := s.DB.Raw("PRAGMA user_version").Scan(&userVersion).Error; err != nil {
		return 0, err
	}
	return userVersion, nil
}

// UpdateUserVersion to the latest
func (s *Source) UpdateUserVersion(version int) error {
	return s.DB.Exec(fmt.Sprintf("PRAGMA user_version=%d", version)).Error
}

func UpperFirst(str string) string {
	return string(unicode.ToUpper(rune(str[0]))) + str[1:]
}

// ColumnExists checks if a column exists in the specified table
func (s *Source) ColumnExists(tableName, columnName string) (bool, error) {
	exists := s.DB.Migrator().HasColumn(tableName, columnName)
	return exists, nil
}
