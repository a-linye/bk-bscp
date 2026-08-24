/*
 * Tencent is pleased to support the open source community by making Blueking Container Service available.
 * Copyright (C) 2019 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

package dao

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

func TestIsDuplicateKeyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		{name: "普通错误", err: errors.New("some error"), want: false},
		{name: "gorm 重复键错误", err: gorm.ErrDuplicatedKey, want: true},
		{name: "包装后的 gorm 重复键错误", err: fmt.Errorf("wrap: %w", gorm.ErrDuplicatedKey), want: true},
		{name: "mysql 1062 错误", err: &mysql.MySQLError{Number: 1062, Message: "Duplicate entry"}, want: true},
		{name: "包装后的 mysql 1062 错误", err: fmt.Errorf("wrap: %w",
			&mysql.MySQLError{Number: 1062, Message: "Duplicate entry"}), want: true},
		{name: "其他 mysql 错误", err: &mysql.MySQLError{Number: 1213, Message: "Deadlock found"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDuplicateKeyError(tt.err); got != tt.want {
				t.Errorf("isDuplicateKeyError() = %v, want %v", got, tt.want)
			}
		})
	}
}
