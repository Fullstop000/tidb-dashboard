// Copyright 2019 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package decorator

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/pingcap/log"
	"go.etcd.io/etcd/clientv3"
	"go.uber.org/zap"

	"github.com/pingcap-incubator/tidb-dashboard/pkg/pd"
)

const (
	SchemaVersionPath = "/tidb/ddl/global_schema_version"
)

type dbInfo struct {
	Name struct {
		O string `json:"O"`
		L string `json:"L"`
	} `json:"db_name"`
	State int `json:"state"`
}

type tableInfo struct {
	ID   int64 `json:"id"`
	Name struct {
		O string `json:"O"`
		L string `json:"L"`
	} `json:"name"`
	Indices []struct {
		ID   int64 `json:"id"`
		Name struct {
			O string `json:"O"`
			L string `json:"L"`
		} `json:"idx_name"`
	} `json:"index_info"`
}

<<<<<<< HEAD
func (s *tidbLabelStrategy) updateAddress(ctx context.Context) {
	cli := s.Provider.EtcdClient
	var info serverInfo
	for i := 0; i < retryCnt; i++ {
		var tidbAddress []string
		ectx, cancel := context.WithTimeout(ctx, etcdGetTimeout)
		resp, err := cli.Get(ectx, pd.TiDBServerInformationPath, clientv3.WithPrefix())
		cancel()
		if err != nil {
			log.Warn("get key failed", zap.String("key", pd.TiDBServerInformationPath), zap.Error(err))
			time.Sleep(200 * time.Millisecond)
			continue
		}
		for _, kv := range resp.Kvs {
			err = json.Unmarshal(kv.Value, &info)
			if err != nil {
				log.Warn("get key failed", zap.String("key", pd.TiDBServerInformationPath), zap.Error(err))
				continue
			}
			tidbAddress = append(tidbAddress, fmt.Sprintf("%s:%d", info.IP, info.StatusPort))
		}
		if len(tidbAddress) > 0 {
			s.TidbAddress = tidbAddress
			break
		}
	}
}

func (s *tidbLabelStrategy) updateMap(ctx context.Context) {
	// check schema version
	ectx, cancel := context.WithTimeout(ctx, etcdGetTimeout)
	resp, err := s.Provider.EtcdClient.Get(ectx, SchemaVersionPath)
	cancel()
	if err != nil || len(resp.Kvs) != 1 {
		log.Warn("failed to get tidb schema version", zap.Error(err))
		return
	}
	schemaVersion, err := strconv.ParseInt(string(resp.Kvs[0].Value), 10, 64)
	if err != nil {
		log.Warn("failed to get tidb schema version", zap.Error(err))
		return
	}
	if schemaVersion == s.SchemaVersion {
		log.Debug("schema version has not changed, skip this update")
		return
	}

	log.Debug("schema version has changed", zap.Int64("old", s.SchemaVersion), zap.Int64("new", schemaVersion))
	s.SchemaVersion = schemaVersion

=======
func (s *tidbLabelStrategy) updateMap() {
>>>>>>> make lint happy
	var dbInfos []*dbInfo
	var tidbEndpoint string
	reqScheme := "http"
	if s.Config.ClusterTLSConfig != nil {
		reqScheme = "https"
	}
	hostname, port := s.forwarder.GetTiDBStatusConnProps()
	target := fmt.Sprintf("%s:%d", hostname, port)
	reqEndpoint := fmt.Sprintf("%s://%s", reqScheme, target)
	if err := request(reqEndpoint, "schema", &dbInfos, s.HTTPClient); err == nil {
		tidbEndpoint = reqEndpoint
	} else {
		log.Error("fail to send schema reqeust to tidb", zap.String("endpoint", reqEndpoint), zap.Error(err))
	}
	if dbInfos == nil {
		return
	}

	var tableInfos []*tableInfo
	for _, db := range dbInfos {
		if db.State == 0 {
			continue
		}
		if err := request(tidbEndpoint, fmt.Sprintf("schema/%s", db.Name.O), &tableInfos, s.HTTPClient); err != nil {
			continue
		}
		for _, table := range tableInfos {
			indices := make(map[int64]string, len(table.Indices))
			for _, index := range table.Indices {
				indices[index.ID] = index.Name.O
			}
			detail := &tableDetail{
				Name:    table.Name.O,
				DB:      db.Name.O,
				ID:      table.ID,
				Indices: indices,
			}
			s.TableMap.Store(table.ID, detail)
		}
	}
}
