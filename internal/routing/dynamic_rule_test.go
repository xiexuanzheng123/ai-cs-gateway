package routing

import "testing"

func TestDynamicRuleMatchSupportsCommonSeparators(t *testing.T) {
	router := NewDynamicRuleRouter()
	router.ReplaceRules([]DynamicRule{
		{
			ID:          1,
			RuleType:    "faq",
			Pattern:     "退款，退钱、退会员\n密码错误 hi",
			Action:      "answer",
			Priority:    1,
			Enabled:     true,
			Description: "matched",
		},
	})

	tests := []string{
		"我要退款",
		"怎么退钱",
		"退会员失败",
		"密码错误太多",
		"hello hi",
	}

	for _, tt := range tests {
		if _, matched := router.Match(tt); !matched {
			t.Fatalf("expected %q to match common separated pattern", tt)
		}
	}
}

func TestDynamicRuleMatchIgnoresDisabledRules(t *testing.T) {
	router := NewDynamicRuleRouter()
	router.ReplaceRules([]DynamicRule{
		{
			ID:          1,
			RuleType:    "faq",
			Pattern:     "退款",
			Action:      "answer",
			Priority:    1,
			Enabled:     false,
			Description: "disabled",
		},
	})

	if _, matched := router.Match("我要退款"); matched {
		t.Fatal("expected disabled rule not to match")
	}
}
