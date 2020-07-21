package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

// PolicyReport describes the result of the kernel policy and the approvers for an event type
type PolicyReport struct {
	Mode      PolicyMode
	Flags     PolicyFlag
	Approvers rules.Approvers
}

// Report describes the event types and their associated policy reports
type Report struct {
	Policies map[string]*PolicyReport
}

// NewReport returns a new report
func NewReport() *Report {
	return &Report{
		Policies: make(map[string]*PolicyReport),
	}
}

// Reporter describes a reporter of policy application
type Reporter struct {
	report *Report
}

func (r *Reporter) getPolicyReport(eventType eval.EventType) *PolicyReport {
	if r.report.Policies[eventType] == nil {
		r.report.Policies[eventType] = &PolicyReport{Approvers: rules.Approvers{}}
	}
	return r.report.Policies[eventType]
}

// ApplyFilterPolicy is called when a passing policy for an event type is applied
func (r *Reporter) ApplyFilterPolicy(eventType eval.EventType, tableName string, mode PolicyMode, flags PolicyFlag) error {
	policyReport := r.getPolicyReport(eventType)
	policyReport.Mode = mode
	policyReport.Flags = flags
	return nil
}

// ApplyApprovers is called when approvers are applied for an event type
func (r *Reporter) ApplyApprovers(eventType eval.EventType, hookPoint *HookPoint, approvers rules.Approvers) error {
	policyReport := r.getPolicyReport(eventType)
	policyReport.Approvers = approvers
	return nil
}

// GetReport returns the report
func (r *Reporter) GetReport() *Report {
	return r.report
}

// NewReporter instantiates a new reporter
func NewReporter() *Reporter {
	return &Reporter{report: NewReport()}
}
