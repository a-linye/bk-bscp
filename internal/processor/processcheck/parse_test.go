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
	"errors"
	"testing"
)

// sampleProcScreen 基准为 specs/stories/135663906/samples/proc-example.json：
// 3 条 contact=GSEKIT_BIZ_100148（nginx_1/2/3）+ 2 条 contact=nodeman（bkmonitorbeat/bksecbeat）。
const sampleBizID = uint32(3)

const sampleProcScreen = `{
    "proc": [
        {
            "procName": "nginx", "setupPath": "/usr/sbin", "pidPath": "/run/nginx-1.pid",
            "contact": "GSEKIT_BIZ_100148",
            "startCmd": "nginx -c /etc/nginx/nginx-1.conf",
            "stopCmd": "nginx -c /etc/nginx/nginx-1.conf -s stop",
            "restartCmd": "nginx -c /etc/nginx/nginx-1.conf -s reload",
            "reloadCmd": "nginx -c /etc/nginx/nginx-1.conf -s reload",
            "killCmd": "kill -9 $(cat /run/nginx-1.pid)",
            "versionCmd": "", "healthCmd": "", "type": 1, "cpulmt": 0, "memlmt": 0,
            "user": "root", "password": "", "userPwd": ":::root@@@",
            "valuekey": "GSEKIT_BIZ_100148:nginx_1",
            "startCheckBeginTime": 10, "startCheckEndTime": 0, "opTimeOut": 10,
            "operateType": 0, "timestamp": 1782360282
        },
        {
            "procName": "nginx", "setupPath": "/usr/sbin", "pidPath": "/run/nginx-2.pid",
            "contact": "GSEKIT_BIZ_100148",
            "startCmd": "nginx -c /etc/nginx/nginx-2.conf",
            "stopCmd": "nginx -c /etc/nginx/nginx-2.conf -s stop",
            "restartCmd": "nginx -c /etc/nginx/nginx-2.conf -s reload",
            "reloadCmd": "nginx -c /etc/nginx/nginx-2.conf -s reload",
            "killCmd": "kill -9 $(cat /run/nginx-2.pid)",
            "versionCmd": "", "healthCmd": "", "type": 1, "cpulmt": 0, "memlmt": 0,
            "user": "root", "password": "", "userPwd": ":::root@@@",
            "valuekey": "GSEKIT_BIZ_100148:nginx_2",
            "startCheckBeginTime": 10, "startCheckEndTime": 0, "opTimeOut": 10,
            "operateType": 0, "timestamp": 1782360279
        },
        {
            "procName": "nginx", "setupPath": "/usr/sbin", "pidPath": "/run/nginx-3.pid",
            "contact": "GSEKIT_BIZ_100148",
            "startCmd": "nginx -c /etc/nginx/nginx-3.conf",
            "stopCmd": "nginx -c /etc/nginx/nginx-3.conf -s stop",
            "restartCmd": "nginx -c /etc/nginx/nginx-3.conf -s reload",
            "reloadCmd": "nginx -c /etc/nginx/nginx-3.conf -s reload",
            "killCmd": "kill -9 $(cat /run/nginx-3.pid)",
            "versionCmd": "", "healthCmd": "", "type": 1, "cpulmt": 0, "memlmt": 0,
            "user": "root", "password": "", "userPwd": ":::root@@@",
            "valuekey": "GSEKIT_BIZ_100148:nginx_3",
            "startCheckBeginTime": 10, "startCheckEndTime": 0, "opTimeOut": 10,
            "operateType": 0, "timestamp": 1782360282
        },
        {
            "procName": "bkmonitorbeat", "setupPath": "/usr/local/gse2_bkte/plugins/bin",
            "pidPath": "/var/run/gse2_bkte/bkmonitorbeat.pid", "contact": "nodeman",
            "startCmd": "./start.sh bkmonitorbeat", "stopCmd": "./stop.sh bkmonitorbeat",
            "restartCmd": "./restart.sh bkmonitorbeat", "reloadCmd": "./reload.sh bkmonitorbeat",
            "killCmd": "", "versionCmd": "./bkmonitorbeat -v", "healthCmd": "",
            "type": 1, "cpulmt": 10, "memlmt": 10, "user": "root", "password": "", "userPwd": ":::root@@@",
            "valuekey": "nodeman:bkmonitorbeat",
            "startCheckBeginTime": 30, "startCheckEndTime": 0, "opTimeOut": 60,
            "operateType": 7, "timestamp": 1772710657
        },
        {
            "procName": "bksecbeat", "setupPath": "/usr/local/gse2_bkte/plugins/bin",
            "pidPath": "/var/run/gse2_bkte/bksecbeat.pid", "contact": "nodeman",
            "startCmd": "./hidsStart.sh", "stopCmd": "./hidsStop.sh",
            "restartCmd": "./hidsRestart.sh", "reloadCmd": "./hidsRestart.sh",
            "killCmd": "", "versionCmd": "./bksecbeat -v", "healthCmd": "",
            "type": 1, "cpulmt": 10, "memlmt": 10, "user": "root", "password": "", "userPwd": ":::root@@@",
            "valuekey": "nodeman:bksecbeat",
            "startCheckBeginTime": 30, "startCheckEndTime": 0, "opTimeOut": 60,
            "operateType": 7, "timestamp": 1781491462
        }
    ]
}`

func TestParseProcScreen_EmptyScreen(t *testing.T) {
	_, err := ParseProcScreen("", sampleBizID)
	if !errors.Is(err, ErrParsing) {
		t.Fatalf("empty screen want ErrParsing, got %v", err)
	}
}

func TestParseProcScreen_NonJSON(t *testing.T) {
	_, err := ParseProcScreen("command not found", sampleBizID)
	if !errors.Is(err, ErrParsing) {
		t.Fatalf("non-json screen want ErrParsing, got %v", err)
	}
}

func TestParseProcScreen_BrokenJSON(t *testing.T) {
	_, err := ParseProcScreen(`{"proc": [ {"valuekey": ] }`, sampleBizID)
	if !errors.Is(err, ErrParsing) {
		t.Fatalf("broken json want ErrParsing, got %v", err)
	}
}

func TestParseProcScreen_AgentNotAvailable(t *testing.T) {
	_, err := ParseProcScreen("agent not available, please check", sampleBizID)
	if !errors.Is(err, ErrAgentException) {
		t.Fatalf("agent screen want ErrAgentException, got %v", err)
	}
}

func TestParseProcScreen_ContactFilter(t *testing.T) {
	procs, err := ParseProcScreen(sampleProcScreen, sampleBizID)
	if err != nil {
		t.Fatalf("parse normal screen failed: %v", err)
	}
	// 2 条 nodeman 被剔除，仅保留 3 条本业务项。
	if len(procs) != 3 {
		t.Fatalf("want 3 biz procs after contact filter, got %d", len(procs))
	}
	for _, p := range procs {
		if p.Contact != "GSEKIT_BIZ_100148" {
			t.Fatalf("unexpected contact retained: %s", p.Contact)
		}
	}
}

func TestParseProcScreen_FieldMapping(t *testing.T) {
	procs, err := ParseProcScreen(sampleProcScreen, sampleBizID)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	var first ActualProc
	for _, p := range procs {
		if p.ValueKey == "GSEKIT_BIZ_100148:nginx_1" {
			first = p
		}
	}
	if first.ValueKey == "" {
		t.Fatal("nginx_1 not found")
	}
	if first.ProcName != "nginx" || first.SetupPath != "/usr/sbin" ||
		first.PidPath != "/run/nginx-1.pid" || first.User != "root" {
		t.Fatalf("identity field mapping wrong: %+v", first)
	}
	if first.StartCmd != "nginx -c /etc/nginx/nginx-1.conf" ||
		first.KillCmd != "kill -9 $(cat /run/nginx-1.pid)" {
		t.Fatalf("control field mapping wrong: %+v", first)
	}
}

func TestParseProcScreen_NoBizProc(t *testing.T) {
	// 仅 nodeman 项时，本业务过滤后为空切片且不报错。
	screen := `{"proc":[{"valuekey":"nodeman:x","contact":"nodeman"}]}`
	procs, err := ParseProcScreen(screen, sampleBizID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(procs) != 0 {
		t.Fatalf("want 0 biz procs, got %d", len(procs))
	}
}
