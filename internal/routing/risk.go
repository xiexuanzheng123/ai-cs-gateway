package routing

import "strings"

type RiskResult struct {
	Intent      string
	RiskLevel   string
	Reply       string
	Suggestions []string
}

type RiskRouter struct{}

func NewRiskRouter() *RiskRouter {
	return &RiskRouter{}
}

func (r *RiskRouter) Match(message string) (RiskResult, bool) {
	normalized := strings.ToLower(strings.TrimSpace(message))

	if containsAny(normalized, []string{"转人工", "人工客服", "真人客服"}) {
		return handoff("human_requested", "您已选择转人工，我会为您接入人工客服。"), true
	}
	if containsAny(normalized, []string{"退款", "退钱", "退费", "退会员"}) {
		return handoff("refund", "退款问题需要人工客服核实订单和支付状态，我已经为您转人工处理。"), true
	}
	if containsAny(normalized, []string{"投诉", "举报客服", "我要举报", "不满意"}) {
		return handoff("complaint", "投诉问题需要人工客服跟进处理，我已经为您转人工。"), true
	}
	if containsAny(normalized, []string{"注销", "删除账号", "改手机号", "换绑", "修改实名"}) {
		return handoff("account_change", "账号资料变更或删除需要人工核验身份，我已经为您转人工处理。"), true
	}
	if containsAny(normalized, []string{"支付争议", "扣款异常", "重复扣款"}) {
		return handoff("payment_dispute", "支付争议需要人工客服核实支付流水，我已经为您转人工处理。"), true
	}

	return RiskResult{}, false
}

func handoff(intent string, reply string) RiskResult {
	return RiskResult{
		Intent:      intent,
		RiskLevel:   "high",
		Reply:       reply,
		Suggestions: []string{"补充问题描述", "上传截图", "等待人工客服"},
	}
}

func containsAny(value string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(value, keyword) {
			return true
		}
	}
	return false
}
