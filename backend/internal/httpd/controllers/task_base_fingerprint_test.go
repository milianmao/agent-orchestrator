package controllers

import "testing"

// TestTaskBaseParticipatesInFingerprint 防止相同客户端请求键在更换 seed 后复用旧任务。
func TestTaskBaseParticipatesInFingerprint(t *testing.T) {
	first := DelegateTaskRequest{ProjectID: "project", Brief: "task", BaseBranch: "main"}
	second := first
	second.BaseBranch = "topic"
	if clientRequestFingerprint("delegate", first, nil) == clientRequestFingerprint("delegate", second, nil) {
		t.Fatal("base branch change did not change delegate fingerprint")
	}
	if clientRequestFingerprint("delegate", first, nil) != clientRequestFingerprint("delegate", first, nil) {
		t.Fatal("identical retry changed fingerprint")
	}
}
