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
	GSEKitInstanceID int64  `json:"gsekit_instance_id"`
	ProcessID        int64  `json:"process_id"`
	TemplateID       int64  `json:"template_id"`
	InstID           int    `json:"inst_id"`
	TemplateName     string `json:"template_name"`
	Reason           string `json:"reason"` // "content_mismatch" / "render_error"
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

// CompareRender performs a comparison between BSCP rendered results and GSEKit stored rendered content
// nolint:funlen,gocyclo
func (m *Migrator) CompareRender(opts CompareRenderOptions) (*CompareRenderReport, error) {
	ctx := context.Background()
	batchSize := m.cfg.Migration.BatchSize

	report := &CompareRenderReport{Success: true}

	// Initialize minimal global config for bkcmdb.New
	// todo：确认GlobalSettings在这里起什么作用
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

	// Create renderer
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

// processInstKey is a composite key for process instance lookup
type processInstKey struct {
	bkProcessID int64
	instID      int
}

// compareRenderForBiz performs render comparison for a single biz
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

	// Phase 1: collect target IDs
	var targetIDs []int64
	if err := m.sourceDB.Raw(collectTargetIDsQuery, bizID).Scan(&targetIDs).Error; err != nil {
		return bizReport, fmt.Errorf("collect target config instance IDs for biz %d failed: %w", bizID, err)
	}
	log.Printf("  Found %d latest released config instances for biz %d", len(targetIDs), bizID)

	if len(targetIDs) == 0 {
		return bizReport, nil
	}

	// Phase 2: fetch all instances
	var allInstances []GSEKitConfigInstance
	for batchStart := 0; batchStart < len(targetIDs); batchStart += batchSize {
		batchEnd := batchStart + batchSize
		if batchEnd > len(targetIDs) {
			batchEnd = len(targetIDs)
		}
		var batch []GSEKitConfigInstance
		if err := m.sourceDB.Where("id IN ?", targetIDs[batchStart:batchEnd]).
			Find(&batch).Error; err != nil {
			return bizReport, fmt.Errorf("fetch config instances for biz %d failed: %w", bizID, err)
		}
		allInstances = append(allInstances, batch...)
	}

	// Collect unique IDs from instances
	configVersionIDs := make(map[int64]bool)
	processIDs := make(map[int64]bool)
	for _, inst := range allInstances {
		configVersionIDs[inst.ConfigVersionID] = true
		processIDs[inst.BkProcessID] = true
	}

	// Batch query config template versions -> versionContentMap
	versionContentMap, err := m.batchFetchVersionContent(uniqueMapKeys(configVersionIDs), batchSize)
	if err != nil {
		return bizReport, fmt.Errorf("batch fetch version content failed: %w", err)
	}

	// Batch query processes -> processMap
	processIDList := uniqueMapKeys(processIDs)
	processMap, err := m.batchFetchProcesses(processIDList, batchSize)
	if err != nil {
		return bizReport, fmt.Errorf("batch fetch processes failed: %w", err)
	}

	// Batch query process instances -> processInstMap
	processInstMap, err := m.batchFetchProcessInsts(processIDList, batchSize)
	if err != nil {
		return bizReport, fmt.Errorf("batch fetch process instances failed: %w", err)
	}

	// Build binding sets for skip logic
	m.buildBindingSets(bizID)

	// Build CMDB lookup maps
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

	// Fetch template names for diff reporting
	templateNameMap := m.fetchTemplateNames(bizID)

	// Per-instance render comparison
	for _, inst := range allInstances {
		bizReport.Total++

		// Skip: template deleted (no version content)
		templateContent, hasVersion := versionContentMap[inst.ConfigVersionID]
		if !hasVersion {
			bizReport.Skipped++
			continue
		}

		// Skip: binding removed
		if !m.isProcessTemplateBound(inst.ConfigTemplateID, inst.BkProcessID) {
			bizReport.Skipped++
			continue
		}

		// Get process info
		proc, hasProc := processMap[inst.BkProcessID]
		if !hasProc {
			bizReport.DataMissing++
			bizReport.Diffs = append(bizReport.Diffs, CompareRenderDiff{
				GSEKitInstanceID: inst.ID,
				ProcessID:        inst.BkProcessID,
				TemplateID:       inst.ConfigTemplateID,
				InstID:           inst.InstID,
				TemplateName:     templateNameMap[inst.ConfigTemplateID],
				Reason:           "render_error",
				ErrorMsg:         "process not found in gsekit_process table",
			})
			continue
		}

		// Get process instance for LocalInstID
		piKey := processInstKey{inst.BkProcessID, inst.InstID}
		procInst, hasProcInst := processInstMap[piKey]
		hostInstSeq := 0
		if hasProcInst {
			hostInstSeq = procInst.LocalInstID
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

		// Decompress GSEKit content
		expected, err := decompressBz2(inst.Content)
		if err != nil {
			bizReport.RenderFailed++
			bizReport.Diffs = append(bizReport.Diffs, CompareRenderDiff{
				GSEKitInstanceID: inst.ID,
				ProcessID:        inst.BkProcessID,
				TemplateID:       inst.ConfigTemplateID,
				InstID:           inst.InstID,
				TemplateName:     templateNameMap[inst.ConfigTemplateID],
				Reason:           "render_error",
				ErrorMsg:         fmt.Sprintf("bz2 decompress failed: %v", err),
			})
			continue
		}

		// Build render context
		params := render.ProcessContextParams{
			ModuleInstSeq: inst.InstID,
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

		// Render with BSCP renderer (timeout is handled internally by the renderer)
		actual, err := renderer.RenderWithContext(ctx, string(templateContent), render.BuildProcessContext(params))

		if err != nil {
			bizReport.RenderFailed++
			bizReport.Diffs = append(bizReport.Diffs, CompareRenderDiff{
				GSEKitInstanceID: inst.ID,
				ProcessID:        inst.BkProcessID,
				TemplateID:       inst.ConfigTemplateID,
				InstID:           inst.InstID,
				TemplateName:     templateNameMap[inst.ConfigTemplateID],
				Reason:           "render_error",
				ErrorMsg:         err.Error(),
			})
			continue
		}

		// Normalize and compare
		expectedStr := strings.TrimRight(string(expected), "\n\r \t")
		actualStr := strings.TrimRight(actual, "\n\r \t")

		if expectedStr == actualStr {
			bizReport.Matched++
		} else {
			bizReport.Mismatched++
			diff := CompareRenderDiff{
				GSEKitInstanceID: inst.ID,
				ProcessID:        inst.BkProcessID,
				TemplateID:       inst.ConfigTemplateID,
				InstID:           inst.InstID,
				TemplateName:     templateNameMap[inst.ConfigTemplateID],
				Reason:           "content_mismatch",
				ExpectedPreview:  truncateStr(expectedStr, 200),
				ActualPreview:    truncateStr(actualStr, 200),
			}
			bizReport.Diffs = append(bizReport.Diffs, diff)

			if opts.ShowDiff {
				printUnifiedDiff(inst.ID, expectedStr, actualStr, opts.DiffContextLines)
			}
		}
	}

	log.Printf("  Biz %d: total=%d matched=%d mismatched=%d render_failed=%d skipped=%d data_missing=%d",
		bizID, bizReport.Total, bizReport.Matched, bizReport.Mismatched,
		bizReport.RenderFailed, bizReport.Skipped, bizReport.DataMissing)

	return bizReport, nil
}

// batchFetchVersionContent fetches template version content in batches
func (m *Migrator) batchFetchVersionContent(versionIDs []int64, batchSize int) (map[int64][]byte, error) {
	result := make(map[int64][]byte)
	for i := 0; i < len(versionIDs); i += batchSize {
		end := i + batchSize
		if end > len(versionIDs) {
			end = len(versionIDs)
		}
		var versions []GSEKitConfigTemplateVersion
		if err := m.sourceDB.Where("config_version_id IN ?", versionIDs[i:end]).
			Find(&versions).Error; err != nil {
			return nil, fmt.Errorf("fetch config template versions failed: %w", err)
		}
		for _, v := range versions {
			result[v.ConfigVersionID] = v.Content
		}
	}
	return result, nil
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

// buildBindingSets populates the Migrator's binding sets for the given biz.
// It clears any previously loaded data so it can be called per-biz safely.
func (m *Migrator) buildBindingSets(bizID uint32) {
	// Clear previous data to support multi-biz iteration
	m.instanceBindSet = make(map[templateProcessKey]bool)
	m.templateBindSet = make(map[templateProcessKey]bool)
	m.processTemplateMap = make(map[int64]int64)

	var bindings []GSEKitConfigTemplateBindingRelationship
	if err := m.sourceDB.Where("bk_biz_id = ?", bizID).Find(&bindings).Error; err != nil {
		log.Printf("  Warning: read binding relationships for biz %d failed: %v", bizID, err)
		return
	}

	for _, b := range bindings {
		key := templateProcessKey{configTemplateID: b.ConfigTemplateID, processID: b.ProcessObjectID}
		switch b.ProcessObjectType {
		case "INSTANCE":
			m.instanceBindSet[key] = true
		case "TEMPLATE":
			m.templateBindSet[key] = true
		}
	}

	// Load process -> process_template mappings
	var ptMappings []struct {
		BkProcessID       int64 `gorm:"column:bk_process_id"`
		ProcessTemplateID int64 `gorm:"column:process_template_id"`
	}
	if err := m.sourceDB.Raw(
		"SELECT bk_process_id, process_template_id FROM gsekit_process WHERE bk_biz_id = ?",
		bizID).Scan(&ptMappings).Error; err != nil {
		log.Printf("  Warning: read process template mappings for biz %d failed: %v", bizID, err)
		return
	}
	for _, pt := range ptMappings {
		m.processTemplateMap[pt.BkProcessID] = pt.ProcessTemplateID
	}
}

// fetchTemplateNames returns a map of config_template_id -> template_name for reporting
func (m *Migrator) fetchTemplateNames(bizID uint32) map[int64]string {
	result := make(map[int64]string)
	var templates []struct {
		ConfigTemplateID int64  `gorm:"column:config_template_id"`
		TemplateName     string `gorm:"column:template_name"`
	}
	if err := m.sourceDB.Raw(
		"SELECT config_template_id, template_name FROM gsekit_configtemplate WHERE bk_biz_id = ?",
		bizID).Scan(&templates).Error; err != nil {
		log.Printf("  Warning: fetch template names failed: %v", err)
		return result
	}
	for _, t := range templates {
		result[t.ConfigTemplateID] = t.TemplateName
	}
	return result
}

// uniqueMapKeys returns unique keys from a map[int64]bool as a sorted slice
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
func printUnifiedDiff(instanceID int64, expected, actual string, contextLines int) {
	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(expected),
		B:        difflib.SplitLines(actual),
		FromFile: fmt.Sprintf("GSEKit (instance %d)", instanceID),
		ToFile:   fmt.Sprintf("BSCP rendered (instance %d)", instanceID),
		Context:  contextLines,
	}
	text, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		log.Printf("  Error generating diff for instance %d: %v", instanceID, err)
		return
	}
	if text != "" {
		fmt.Printf("\n--- Diff for instance %d ---\n%s\n", instanceID, text)
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
				fmt.Printf("    - Instance %d (process=%d, template=%d/%s, inst=%d): %s",
					d.GSEKitInstanceID, d.ProcessID, d.TemplateID, d.TemplateName, d.InstID, d.Reason)
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
