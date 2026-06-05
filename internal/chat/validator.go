package chat

import "strings"

const validatorFallbackReply = "抱歉，当前回答依据不足。您可以换个方式描述问题，或选择转人工客服为您处理。"

type responseValidationInput struct {
	UserMessage string
	Response    SendMessageResponse
	RAGMatches  []RAGSearchResult
}

type responseValidationResult struct {
	Passed bool
	Action string
	Reason string
}

func validateResponse(input responseValidationInput) responseValidationResult {
	reply := strings.TrimSpace(input.Response.Content.Text)
	if reply == "" {
		return validationFailed("fallback", "empty_reply")
	}
	if isLowQualityReply(reply) {
		return validationFailed("fallback", "low_quality_reply")
	}
	if needsCitation(input.Response.Route) && len(input.Response.Citations) == 0 {
		return validationFailed("fallback", "missing_citation")
	}
	if hasSensitiveUserIntent(input.UserMessage) || hasSensitivePromise(reply) {
		return validationFailed("handoff", "sensitive_intent_or_promise")
	}
	return responseValidationResult{
		Passed: true,
		Action: "pass",
		Reason: "ok",
	}
}

func validationFailed(action string, reason string) responseValidationResult {
	return responseValidationResult{
		Passed: false,
		Action: action,
		Reason: reason,
	}
}

func needsCitation(route string) bool {
	route = strings.ToLower(strings.TrimSpace(route))
	return route == "rag" || route == "rag_llm"
}

func isLowQualityReply(reply string) bool {
	if len([]rune(reply)) < 8 {
		return true
	}
	normalized := strings.ToLower(reply)
	lowQualityReplies := []string{
		"不知道",
		"无法回答",
		"请联系客服",
		"建议咨询客服",
		"我不清楚",
	}
	for _, keyword := range lowQualityReplies {
		if strings.Contains(normalized, keyword) && len([]rune(reply)) <= 24 {
			return true
		}
	}
	return false
}

func hasSensitiveUserIntent(message string) bool {
	normalized := strings.ToLower(message)
	return containsAnyText(normalized, []string{
		"退款",
		"退钱",
		"退费",
		"投诉",
		"举报客服",
		"重复扣款",
		"扣款异常",
		"注销账号",
		"删除账号",
		"修改实名",
		"换绑",
	})
}

func hasSensitivePromise(reply string) bool {
	normalized := strings.ToLower(reply)
	return containsAnyText(normalized, []string{
		"一定退款",
		"肯定退款",
		"一定到账",
		"肯定到账",
		"马上到账",
		"立即到账",
		"一定解封",
		"肯定解封",
		"一定恢复",
	})
}

func containsAnyText(value string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(value, keyword) {
			return true
		}
	}
	return false
}
