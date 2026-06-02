package routing

import (
	"strings"
	"sync"
)

type DynamicRule struct {
	ID          int64
	RuleType    string
	Pattern     string
	Action      string
	Priority    int
	Enabled     bool
	Description string
}

type DynamicRuleRouter struct {
	mu    sync.RWMutex
	rules []DynamicRule
}

func NewDynamicRuleRouter() *DynamicRuleRouter {
	return &DynamicRuleRouter{}
}

func (r *DynamicRuleRouter) ReplaceRules(rules []DynamicRule) {
	enabledRules := make([]DynamicRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Enabled {
			enabledRules = append(enabledRules, rule)
		}
	}

	r.mu.Lock()
	r.rules = enabledRules
	r.mu.Unlock()
}

func (r *DynamicRuleRouter) Match(message string) (RiskResult, bool) {
	normalized := strings.ToLower(strings.TrimSpace(message))
	if normalized == "" {
		return RiskResult{}, false
	}

	r.mu.RLock()
	rules := append([]DynamicRule(nil), r.rules...)
	r.mu.RUnlock()

	for _, rule := range rules {
		if !matchPattern(normalized, rule.Pattern) {
			continue
		}
		return ruleResult(rule), true
	}
	return RiskResult{}, false
}

func matchPattern(message string, pattern string) bool {
	for _, keyword := range strings.Split(pattern, ",") {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword != "" && strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}

func ruleResult(rule DynamicRule) RiskResult {
	reply := strings.TrimSpace(rule.Description)
	if reply == "" {
		reply = "您好，这个问题我已经记录，会继续为您处理。"
	}

	result := RiskResult{
		Intent:      rule.RuleType,
		Route:       "db_rule",
		RiskLevel:   riskLevel(rule),
		Reply:       reply,
		Suggestions: suggestions(rule),
	}
	if rule.Action == "handoff" {
		result.TransferToHuman = true
	}
	return result
}

func riskLevel(rule DynamicRule) string {
	if rule.Action == "handoff" || rule.RuleType == "risk" {
		return "high"
	}
	return "low"
}

func suggestions(rule DynamicRule) []string {
	if rule.Action == "handoff" {
		return []string{"补充问题描述", "上传截图", "等待人工客服"}
	}
	return []string{"继续描述问题", "转人工"}
}
