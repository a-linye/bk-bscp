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

package migrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/TencentBlueKing/bk-bscp/internal/components/bkcmdb"
	processorCmdb "github.com/TencentBlueKing/bk-bscp/internal/processor/cmdb"
	"github.com/TencentBlueKing/bk-bscp/pkg/cc"
	"github.com/TencentBlueKing/bk-bscp/render"
	"github.com/pmezard/go-difflib/difflib"
)

// CompareRenderReport contains the overall comparison report
type CompareRenderReport struct {
	Success    bool               `json:"success"`
	BizReports []BizCompareReport `json:"biz_reports"`
}

// BizCompareReport contains comparison results for a single biz
type BizCompareReport struct {
	BizID        uint32              `json:"biz_id"`
	Total        int                 `json:"total"`
	Matched      int                 `json:"matched"`
	Mismatched   int                 `json:"mismatched"`
	RenderFailed int                 `json:"render_failed"`
	Skipped      int                 `json:"skipped"`
	DataMissing  int                 `json:"data_missing"`
	Diffs        []CompareRenderDiff `json:"diffs,omitempty"`
}

// CompareRenderDiff contains details of a single mismatched comparison
type CompareRenderDiff struct {
	ConfigTemplateID int64  `json:"config_template_id"`
	ConfigVersionID  int64  `json:"config_version_id"`
	ProcessID        int64  `json:"process_id"`
	TemplateName     string `json:"template_name"`
	Reason           string `json:"reason"` // "content_mismatch" / "render_error" / "gsekit_render_error"
	ExpectedPreview  string `json:"expected_preview,omitempty"`
	ActualPreview    string `json:"actual_preview,omitempty"`
	ErrorMsg         string `json:"error_msg,omitempty"`
}

// CompareRenderOptions holds options for compare-render command
type CompareRenderOptions struct {
	ShowDiff         bool
	DiffContextLines int
	OutputFile       string
	RenderTimeout    time.Duration
}

// CompareRender performs a per-template render comparison:
//  1. Query each config template's latest version (active non-draft)
//  2. Pick any one bound process for the template
//  3. Call GSEKit preview API to get GSEKit's rendered result
//  4. Use BSCP renderer to render with the same process context
//  5. Compare the two rendered results
//
// nolint:funlen,gocyclo
func (m *Migrator) CompareRender(opts CompareRenderOptions) (*CompareRenderReport, error) {
	ctx := context.Background()
	batchSize := m.cfg.Migration.BatchSize

	report := &CompareRenderReport{Success: true}

	// Initialize minimal global config for bkcmdb.New
	cc.SetG(cc.GlobalSettings{
		FeatureFlags: cc.FeatureFlags{EnableMultiTenantMode: false},
	})

	// Create bkcmdb.Service for CC XML
	cmdbSvc, err := bkcmdb.New(&cc.CMDBConfig{
		Host:       m.cfg.CMDB.Endpoint,
		AppCode:    m.cfg.CMDB.AppCode,
		AppSecret:  m.cfg.CMDB.AppSecret,
		BkUserName: m.cfg.CMDB.Username,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("create bkcmdb service failed: %w", err)
	}

	// Create BSCP renderer
	renderer, err := render.NewRenderer(render.WithTimeout(opts.RenderTimeout))
	if err != nil {
		return nil, fmt.Errorf("create renderer failed: %w", err)
	}

	for _, bizID := range m.cfg.Migration.BizIDs {
		log.Printf("=== Comparing render results for biz %d ===", bizID)

		bizReport, err := m.compareRenderForBiz(ctx, bizID, batchSize, renderer, cmdbSvc, opts)
		if err != nil {
			log.Printf("  Error comparing biz %d: %v", bizID, err)
			report.Success = false
			bizReport = &BizCompareReport{BizID: bizID}
		}

		if bizReport.Mismatched > 0 || bizReport.RenderFailed > 0 {
			report.Success = false
		}

		report.BizReports = append(report.BizReports, *bizReport)
	}

	return report, nil
}

// templateWithVersion holds a config template joined with its latest version
type templateWithVersion struct {
	Template GSEKitConfigTemplate
	Version  GSEKitConfigTemplateVersion
}

// compareRenderForBiz performs render comparison for a single biz.
// For each config template, it picks any one bound process, renders via both
// GSEKit preview API and BSCP renderer, then compares the results.
// nolint:funlen,gocyclo
func (m *Migrator) compareRenderForBiz(
	ctx context.Context,
	bizID uint32,
	batchSize int,
	renderer *render.Renderer,
	cmdbSvc bkcmdb.Service,
	opts CompareRenderOptions,
) (*BizCompareReport, error) {
	bizReport := &BizCompareReport{BizID: bizID}

	// Step 1: Fetch all config templates for this biz
	var templates []GSEKitConfigTemplate
	if err := m.sourceDB.Where("bk_biz_id = ?", bizID).Find(&templates).Error; err != nil {
		return bizReport, fmt.Errorf("fetch config templates for biz %d failed: %w", bizID, err)
	}
	log.Printf("  Found %d config templates for biz %d", len(templates), bizID)

	if len(templates) == 0 {
		return bizReport, nil
	}

	// Step 2: For each template, find its latest non-draft version (highest config_version_id with is_draft=false)
	templateVersionMap := make(map[int64]*templateWithVersion) // config_template_id -> templateWithVersion
	for _, tmpl := range templates {
		var version GSEKitConfigTemplateVersion
		err := m.sourceDB.Where("config_template_id = ? AND is_draft = ?", tmpl.ConfigTemplateID, false).
			Order("config_version_id DESC").
			First(&version).Error
		if err != nil {
			// No published version, skip
			continue
		}
		templateVersionMap[tmpl.ConfigTemplateID] = &templateWithVersion{
			Template: tmpl,
			Version:  version,
		}
	}
	log.Printf("  %d templates have published versions", len(templateVersionMap))

	if len(templateVersionMap) == 0 {
		return bizReport, nil
	}

	// Step 3: Load binding relationships to find a bound process per template
	var bindings []GSEKitConfigTemplateBindingRelationship
	if err := m.sourceDB.Where("bk_biz_id = ?", bizID).Find(&bindings).Error; err != nil {
		return bizReport, fmt.Errorf("read binding relationships for biz %d failed: %w", bizID, err)
	}

	// Load process -> process_template mappings for TEMPLATE-type binding resolution
	processTemplateMap := make(map[int64]int64) // bk_process_id -> process_template_id
	var ptMappings []struct {
		BkProcessID       int64 `gorm:"column:bk_process_id"`
		ProcessTemplateID int64 `gorm:"column:process_template_id"`
	}
	if err := m.sourceDB.Raw(
		"SELECT bk_process_id, process_template_id FROM gsekit_process WHERE bk_biz_id = ?",
		bizID).Scan(&ptMappings).Error; err != nil {
		return bizReport, fmt.Errorf("read process template mappings for biz %d failed: %w", bizID, err)
	}
	for _, pt := range ptMappings {
		processTemplateMap[pt.BkProcessID] = pt.ProcessTemplateID
	}

	// Build reverse lookup: process_template_id -> [bk_process_id]
	reverseProcessTemplate := make(map[int64][]int64)
	for processID, ptID := range processTemplateMap {
		if ptID != 0 {
			reverseProcessTemplate[ptID] = append(reverseProcessTemplate[ptID], processID)
		}
	}

	// For each config_template, pick any one bound bk_process_id
	// INSTANCE binding: process_object_id IS a bk_process_id
	// TEMPLATE binding: process_object_id is a process_template_id, need reverse lookup
	templateProcessPick := make(map[int64]int64) // config_template_id -> bk_process_id
	for _, b := range bindings {
		if _, already := templateProcessPick[b.ConfigTemplateID]; already {
			continue // already picked one
		}
		switch b.ProcessObjectType {
		case "INSTANCE":
			templateProcessPick[b.ConfigTemplateID] = b.ProcessObjectID
		case "TEMPLATE":
			if procs, ok := reverseProcessTemplate[b.ProcessObjectID]; ok && len(procs) > 0 {
				templateProcessPick[b.ConfigTemplateID] = procs[0]
			}
		}
	}

	// Step 4: Collect all picked process IDs, batch-fetch process info and CMDB data
	processIDSet := make(map[int64]bool)
	for _, pid := range templateProcessPick {
		processIDSet[pid] = true
	}
	processIDList := uniqueMapKeys(processIDSet)

	processMap, err := m.batchFetchProcesses(processIDList, batchSize)
	if err != nil {
		return bizReport, fmt.Errorf("batch fetch processes failed: %w", err)
	}

	// Fetch process instances for LocalInstID (pick inst_id=1 or first available)
	processInstMap, err := m.batchFetchProcessInsts(processIDList, batchSize)
	if err != nil {
		return bizReport, fmt.Errorf("batch fetch process instances failed: %w", err)
	}

	// Collect CMDB IDs from processes
	setIDs := make([]int64, 0)
	moduleIDs := make([]int64, 0)
	svcInstIDs := make([]int64, 0)
	cmdbProcessIDs := make([]int64, 0)
	for _, proc := range processMap {
		setIDs = append(setIDs, proc.BkSetID)
		moduleIDs = append(moduleIDs, proc.BkModuleID)
		svcInstIDs = append(svcInstIDs, proc.ServiceInstanceID)
		cmdbProcessIDs = append(cmdbProcessIDs, proc.BkProcessID)
	}

	setNames, err := m.cmdbClient.FindSetBatch(ctx, bizID, uniqueInt64(setIDs))
	if err != nil {
		log.Printf("  Warning: FindSetBatch failed: %v", err)
		setNames = make(map[int64]string)
	}

	moduleNames, err := m.cmdbClient.FindModuleBatch(ctx, bizID, uniqueInt64(moduleIDs))
	if err != nil {
		log.Printf("  Warning: FindModuleBatch failed: %v", err)
		moduleNames = make(map[int64]string)
	}

	svcInstDetails, err := m.cmdbClient.ListServiceInstanceDetail(ctx, bizID, uniqueInt64(svcInstIDs))
	if err != nil {
		log.Printf("  Warning: ListServiceInstanceDetail failed: %v", err)
		svcInstDetails = make(map[int64]*CMDBServiceInstance)
	}
	svcInstNames := make(map[int64]string)
	for id, detail := range svcInstDetails {
		svcInstNames[id] = detail.Name
	}

	processDetails, err := m.cmdbClient.ListProcessDetailByIds(ctx, bizID, uniqueInt64(cmdbProcessIDs))
	if err != nil {
		log.Printf("  Warning: ListProcessDetailByIds failed: %v", err)
		processDetails = make(map[int64]*CMDBProcessDetail)
	}

	// Build CC XML (once per biz)
	topoSvc := processorCmdb.NewCCTopoXMLService(int(bizID), cmdbSvc)
	ccXML, err := topoSvc.GetTopoTreeXML(ctx, "")
	if err != nil {
		log.Printf("  Warning: GetTopoTreeXML failed for biz %d: %v", bizID, err)
		ccXML = ""
	}

	globalVars, err := topoSvc.GetBizGlobalVariablesMap(ctx)
	if err != nil {
		log.Printf("  Warning: GetBizGlobalVariablesMap failed for biz %d: %v", bizID, err)
		globalVars = make(map[string]interface{})
	}

	// Step 5: Per-template render comparison
	for configTemplateID, tv := range templateVersionMap {
		bizReport.Total++

		// Check if we have a bound process for this template
		processID, hasBound := templateProcessPick[configTemplateID]
		if !hasBound {
			bizReport.Skipped++
			log.Printf("  Skipped template %d (%s): no bound process",
				configTemplateID, tv.Template.TemplateName)
			continue
		}

		proc, hasProc := processMap[processID]
		if !hasProc {
			bizReport.DataMissing++
			bizReport.Diffs = append(bizReport.Diffs, CompareRenderDiff{
				ConfigTemplateID: configTemplateID,
				ConfigVersionID:  tv.Version.ConfigVersionID,
				ProcessID:        processID,
				TemplateName:     tv.Template.TemplateName,
				Reason:           "render_error",
				ErrorMsg:         "bound process not found in gsekit_process table",
			})
			continue
		}

		// Pick any one process instance for HostInstSeq (prefer inst_id=1)
		instID := 1
		hostInstSeq := 0
		piKey := processInstKey{processID, instID}
		if procInst, ok := processInstMap[piKey]; ok {
			hostInstSeq = procInst.LocalInstID
		} else {
			// Try to find any process instance for this process
			for key, procInst := range processInstMap {
				if key.bkProcessID == processID {
					instID = key.instID
					hostInstSeq = procInst.LocalInstID
					break
				}
			}
		}

		// Get CMDB process detail
		detail := processDetails[proc.BkProcessID]
		funcName := ""
		workPath := ""
		pidFile := ""
		if detail != nil {
			funcName = detail.BkFuncName
			workPath = detail.WorkPath
			pidFile = detail.PidFile
		}

		// --- GSEKit side: call preview API ---
		// TODO: 调用 GSEKit 配置模版预览接口获取渲染结果
		// 参数: bizID, configTemplateID, tv.Version.ConfigVersionID, processID, instID
		// 返回: gsekitRendered (string), err
		// 接口由后续提供，当前先置为空字符串
		gsekitRendered := ""
		_ = bizID
		var gsekitRenderErr error
		// TODO: gsekitRendered, gsekitRenderErr = m.gsekitClient.PreviewConfigTemplate(ctx, bizID, configTemplateID, tv.Version.ConfigVersionID, processID, instID)

		if gsekitRenderErr != nil {
			bizReport.RenderFailed++
			bizReport.Diffs = append(bizReport.Diffs, CompareRenderDiff{
				ConfigTemplateID: configTemplateID,
				ConfigVersionID:  tv.Version.ConfigVersionID,
				ProcessID:        processID,
				TemplateName:     tv.Template.TemplateName,
				Reason:           "gsekit_render_error",
				ErrorMsg:         gsekitRenderErr.Error(),
			})
			continue
		}

		// --- BSCP side: render with BSCP renderer ---
		templateContent := string(tv.Version.Content)
		params := render.ProcessContextParams{
			ModuleInstSeq: instID,
			HostInstSeq:   hostInstSeq,
			SetName:       setNames[proc.BkSetID],
			ModuleName:    moduleNames[proc.BkModuleID],
			ServiceName:   svcInstNames[proc.ServiceInstanceID],
			ProcessName:   proc.BkProcessName,
			ProcessID:     int(proc.BkProcessID),
			FuncName:      funcName,
			WorkPath:      workPath,
			PidFile:       pidFile,
			HostInnerIP:   proc.BkHostInnerip,
			CloudID:       int(proc.BkCloudID),
			CcXML:         ccXML,
			GlobalVariables: map[string]interface{}{
				"biz_global_variables": globalVars,
			},
		}

		bscpRendered, err := renderer.RenderWithContext(ctx, templateContent, render.BuildProcessContext(params))
		if err != nil {
			bizReport.RenderFailed++
			bizReport.Diffs = append(bizReport.Diffs, CompareRenderDiff{
				ConfigTemplateID: configTemplateID,
				ConfigVersionID:  tv.Version.ConfigVersionID,
				ProcessID:        processID,
				TemplateName:     tv.Template.TemplateName,
				Reason:           "render_error",
				ErrorMsg:         err.Error(),
			})
			continue
		}

		// Normalize and compare
		expectedStr := strings.TrimRight(gsekitRendered, "\n\r \t")
		actualStr := strings.TrimRight(bscpRendered, "\n\r \t")

		if expectedStr == actualStr {
			bizReport.Matched++
		} else {
			bizReport.Mismatched++
			diff := CompareRenderDiff{
				ConfigTemplateID: configTemplateID,
				ConfigVersionID:  tv.Version.ConfigVersionID,
				ProcessID:        processID,
				TemplateName:     tv.Template.TemplateName,
				Reason:           "content_mismatch",
				ExpectedPreview:  truncateStr(expectedStr, 200),
				ActualPreview:    truncateStr(actualStr, 200),
			}
			bizReport.Diffs = append(bizReport.Diffs, diff)

			if opts.ShowDiff {
				printUnifiedDiff(configTemplateID, expectedStr, actualStr, opts.DiffContextLines)
			}
		}
	}

	log.Printf("  Biz %d: total=%d matched=%d mismatched=%d render_failed=%d skipped=%d data_missing=%d",
		bizID, bizReport.Total, bizReport.Matched, bizReport.Mismatched,
		bizReport.RenderFailed, bizReport.Skipped, bizReport.DataMissing)

	return bizReport, nil
}

// processInstKey is a composite key for process instance lookup
type processInstKey struct {
	bkProcessID int64
	instID      int
}

// batchFetchProcesses fetches processes in batches
func (m *Migrator) batchFetchProcesses(processIDs []int64, batchSize int) (map[int64]*GSEKitProcess, error) {
	result := make(map[int64]*GSEKitProcess)
	for i := 0; i < len(processIDs); i += batchSize {
		end := i + batchSize
		if end > len(processIDs) {
			end = len(processIDs)
		}
		var procs []GSEKitProcess
		if err := m.sourceDB.Where("bk_process_id IN ?", processIDs[i:end]).
			Find(&procs).Error; err != nil {
			return nil, fmt.Errorf("fetch processes failed: %w", err)
		}
		for idx := range procs {
			result[procs[idx].BkProcessID] = &procs[idx]
		}
	}
	return result, nil
}

// batchFetchProcessInsts fetches process instances in batches and returns a map keyed by (bk_process_id, inst_id)
func (m *Migrator) batchFetchProcessInsts(processIDs []int64, batchSize int) (map[processInstKey]*GSEKitProcessInst, error) {
	result := make(map[processInstKey]*GSEKitProcessInst)
	for i := 0; i < len(processIDs); i += batchSize {
		end := i + batchSize
		if end > len(processIDs) {
			end = len(processIDs)
		}
		var insts []GSEKitProcessInst
		if err := m.sourceDB.Where("bk_process_id IN ?", processIDs[i:end]).
			Find(&insts).Error; err != nil {
			return nil, fmt.Errorf("fetch process instances failed: %w", err)
		}
		for idx := range insts {
			key := processInstKey{insts[idx].BkProcessID, insts[idx].InstID}
			result[key] = &insts[idx]
		}
	}
	return result, nil
}

// uniqueMapKeys returns keys from a map[int64]bool as a slice
func uniqueMapKeys(m map[int64]bool) []int64 {
	result := make([]int64, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}

// truncateStr truncates a string to maxLen characters, appending "..." if truncated
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// printUnifiedDiff prints a unified diff between expected and actual content
func printUnifiedDiff(templateID int64, expected, actual string, contextLines int) {
	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(expected),
		B:        difflib.SplitLines(actual),
		FromFile: fmt.Sprintf("GSEKit (template %d)", templateID),
		ToFile:   fmt.Sprintf("BSCP rendered (template %d)", templateID),
		Context:  contextLines,
	}
	text, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		log.Printf("  Error generating diff for template %d: %v", templateID, err)
		return
	}
	if text != "" {
		fmt.Printf("\n--- Diff for template %d ---\n%s\n", templateID, text)
	}
}

// PrintCompareRenderReport prints the comparison report to stdout in table format
func (m *Migrator) PrintCompareRenderReport(report *CompareRenderReport) {
	fmt.Println("\n========== Compare Render Report ==========")
	fmt.Printf("Status: %s\n", boolToStatus(report.Success))

	for _, biz := range report.BizReports {
		fmt.Printf("\nBiz %d:\n", biz.BizID)
		fmt.Printf("  Total:         %d\n", biz.Total)
		fmt.Printf("  Matched:       %d\n", biz.Matched)
		fmt.Printf("  Mismatched:    %d\n", biz.Mismatched)
		fmt.Printf("  Render Failed: %d\n", biz.RenderFailed)
		fmt.Printf("  Skipped:       %d\n", biz.Skipped)
		fmt.Printf("  Data Missing:  %d\n", biz.DataMissing)

		if len(biz.Diffs) > 0 {
			fmt.Printf("\n  Differences (%d):\n", len(biz.Diffs))
			for _, d := range biz.Diffs {
				fmt.Printf("    - Template %d/%s (version=%d, process=%d): %s",
					d.ConfigTemplateID, d.TemplateName, d.ConfigVersionID, d.ProcessID, d.Reason)
				if d.ErrorMsg != "" {
					fmt.Printf(" [%s]", d.ErrorMsg)
				}
				fmt.Println()
			}
		}
	}
	fmt.Println("=============================================")
}

// WriteCompareRenderReportJSON writes the report as JSON to the specified file
func (m *Migrator) WriteCompareRenderReportJSON(report *CompareRenderReport, outputFile string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report to JSON failed: %w", err)
	}
	if err := os.WriteFile(outputFile, data, 0644); err != nil {
		return fmt.Errorf("write report to %s failed: %w", outputFile, err)
	}
	log.Printf("Report written to %s", outputFile)
	return nil
}
