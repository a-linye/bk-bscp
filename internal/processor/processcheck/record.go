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

package processcheck

import (
	"github.com/TencentBlueKing/bk-bscp/internal/dal/dao"
	"github.com/TencentBlueKing/bk-bscp/pkg/dal/table"
	"github.com/TencentBlueKing/bk-bscp/pkg/kit"
)

// ApplyResult 把单实例检查结论落到上游异常记录存储：
//   - exception：追加写入一条 status=exception 记录（host_id 取自 Process.Attachment.HostID）；
//   - pass：最新记录为 exception 时（IsException==true）取最新记录并 UpdateStatus(recovered) 完成闭环，否则不动作；
//   - skip：无任何写入。
//
// 以「最近一次检查结论」为准（FR-011）。写库错误透传给调用方，由编排层记日志并在下一轮重试，不阻断其余实例。
func ApplyResult(kt *kit.Kit, store dao.ProcessManagedException, r CheckResult) error {
	switch r.Verdict {
	case VerdictException:
		m := &table.ProcessManagedException{
			Attachment: &table.ProcessManagedExceptionAttachment{
				TenantID:          r.TenantID,
				BizID:             r.BizID,
				HostID:            r.HostID,
				ProcessID:         r.ProcessID,
				ProcessInstanceID: r.ProcessInstanceID,
			},
			Spec: &table.ProcessManagedExceptionSpec{
				ErrorType:          r.ErrorType,
				ErrorMsg:           r.ErrorMsg,
				HandlingSuggestion: r.HandlingSuggestion,
				Status:             table.ProcessExceptionStatusException,
				CheckedAt:          r.CheckedAt,
			},
		}
		_, err := store.Create(kt, m)
		return err

	case VerdictPass:
		isExc, err := store.IsException(kt, r.BizID, r.ProcessInstanceID)
		if err != nil {
			return err
		}
		if !isExc {
			return nil
		}
		latest, err := store.GetLatestByProcessInstanceID(kt, r.BizID, r.ProcessInstanceID)
		if err != nil {
			return err
		}
		return store.UpdateStatus(kt, r.BizID, latest.ID, table.ProcessExceptionStatusRecovered)

	default:
		// VerdictSkip：无写入、无状态更新。
		return nil
	}
}
