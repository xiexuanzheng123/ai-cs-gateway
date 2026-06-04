package routing

import "strings"

type RiskResult struct {
	Intent          string
	Route           string
	RiskLevel       string
	Reply           string
	TransferToHuman bool
	Suggestions     []string
}

type RiskRouter struct{}

func NewRiskRouter() *RiskRouter {
	return &RiskRouter{}
}

func (r *RiskRouter) Match(message string) (RiskResult, bool) {
	normalized := strings.ToLower(strings.TrimSpace(message))

	// 内置规则是最后的安全网；运营规则可覆盖更多细节，但高风险词不能完全依赖后台配置。
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
	if containsAny(normalized, []string{"密码错误过多", "密码错误太多"}) {
		return fixedFAQ(
			"faq_password_too_many_failures",
			"密码错误次数过多时，建议您先等待一段时间后再尝试登录，或通过找回账号密码重新设置密码。如果仍无法登录，可以继续转人工处理。",
			[]string{"找回账号密码", "账号异常设备登录", "转人工"},
		), true
	}
	if containsAny(normalized, []string{"找回账号密码", "找回密码", "忘记密码"}) {
		return fixedFAQ(
			"faq_recover_account_password",
			"您可以在登录页选择找回账号或忘记密码，根据手机号、绑定信息或身份校验流程完成账号密码找回。",
			[]string{"密码错误过多", "账号异常设备登录", "转人工"},
		), true
	}
	if containsAny(normalized, []string{"账号异常设备登录", "异常设备登录", "异地登录"}) {
		return fixedFAQ(
			"faq_abnormal_device_login",
			"如果发现账号存在异常设备登录，建议您尽快修改密码，并检查绑定手机号和登录设备。如有资金或账号安全风险，可以转人工处理。",
			[]string{"修改密码", "账号安全", "转人工"},
		), true
	}
	if containsAny(normalized, []string{"账号违规举报", "违规举报", "举报账号"}) {
		return fixedFAQ(
			"faq_report_account_violation",
			"账号违规举报需要提供被举报账号、违规说明和相关截图。您提交后会进入人工审核流程。",
			[]string{"补充截图", "继续描述问题", "转人工"},
		), true
	}

	return RiskResult{}, false
}

func handoff(intent string, reply string) RiskResult {
	return RiskResult{
		Intent:          intent,
		Route:           "rule_handoff",
		RiskLevel:       "high",
		Reply:           reply,
		TransferToHuman: true,
		Suggestions:     []string{"补充问题描述", "上传截图", "等待人工客服"},
	}
}

func fixedFAQ(intent string, reply string, suggestions []string) RiskResult {
	return RiskResult{
		Intent:          intent,
		Route:           "fixed_faq",
		RiskLevel:       "low",
		Reply:           reply,
		TransferToHuman: false,
		Suggestions:     suggestions,
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
