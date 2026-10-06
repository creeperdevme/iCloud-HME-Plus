package account

import (
	"encoding/json"
	"net/mail"
	"strings"
	"testing"
)

// TestSummaryDoesNotSerializeSecrets 驗證 Summary 序列化後不包含任何秘密。
func TestSummaryDoesNotSerializeSecrets(t *testing.T) {
	acc := &Account{
		ID:          "acc_test",
		Name:        "主號",
		Cookies:     map[string]string{"token": "cookie-secret"},
		AppPassword: "app-secret",
		Proxy:       "http://user:proxy-secret@example.com:8080",
	}
	summary := acc.Summary()
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"cookie-secret", "app-secret", "proxy-secret"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("回應洩露秘密 %q: %s", secret, raw)
		}
	}
	if !summary.HasCookies || !summary.HasAppPassword || !summary.HasProxy {
		t.Fatalf("憑證狀態錯誤: %+v", summary)
	}
}

// TestSummaryMapsStatusMessage 驗證 status_message 使用固定文案,不洩露 LastError。
func TestSummaryMapsStatusMessage(t *testing.T) {
	cases := []struct {
		status string
		want   string
	}{
		{"pending", "等待設定或驗證憑證"},
		{"error", "憑證驗證失敗"},
		{"active", ""},
	}
	for _, tc := range cases {
		acc := &Account{ID: "acc_" + tc.status, Status: tc.status, LastError: "內部錯誤: http://user:secret@host"}
		if got := acc.Summary().StatusMessage; got != tc.want {
			t.Errorf("status=%s 期望 StatusMessage=%q 得到 %q", tc.status, tc.want, got)
		}
	}
}

// TestAddAccountWithInputValidation 驗證新增帳號輸入校驗。
func TestAddAccountWithInputValidation(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		input AddAccountInput
		want  string // 期望錯誤片段;空表示成功
	}{
		{"空名稱自動推導", AddAccountInput{Name: "   ", ICloudEmail: "a@icloud.com"}, ""},
		{"完全省略名稱", AddAccountInput{ICloudEmail: "a@icloud.com"}, ""},
		{"名稱過長", AddAccountInput{Name: strings.Repeat("名", 65), ICloudEmail: "a@icloud.com"}, "名稱"},
		{"非法主機", AddAccountInput{Name: "主號", ICloudEmail: "a@icloud.com", Host: "evil.com"}, "主機"},
		{"非法前綴", AddAccountInput{Name: "主號", ICloudEmail: "bad prefix"}, "信箱"},
		{"只填 Prefix", AddAccountInput{ICloudEmail: "owner"}, ""},
		{"非法代理", AddAccountInput{Name: "主號", ICloudEmail: "a@icloud.com", Proxy: "ftp://user:pass@host:21"}, "代理"},
		{"合法最小輸入", AddAccountInput{Name: "主號", ICloudEmail: "a@icloud.com"}, ""},
	}
	for _, tc := range cases {
		_, err := m.AddAccountWithInput(tc.input)
		if tc.want == "" {
			if err != nil {
				t.Errorf("%s: 期望成功,得到 %v", tc.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: 期望錯誤包含 %q,得到 %v", tc.name, tc.want, err)
		}
	}
}

// TestAddAccountWithInputValidatesEmailFormat 驗證信箱必須等於解析後的位址。
func TestAddAccountWithInputValidatesEmailFormat(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.AddAccountWithInput(AddAccountInput{Name: "主號", ICloudEmail: `"Quoted" <a@icloud.com>`})
	if err == nil {
		t.Fatal("期望帶顯示名的信箱被拒絕")
	}
	addr, err := mail.ParseAddress("a@icloud.com")
	if err != nil || addr.Address != "a@icloud.com" {
		t.Fatalf("測試前置錯誤: %v %q", err, addr)
	}
}

// TestUpdateMetadataValidation 驗證編輯帳號校驗與行為。
func TestUpdateMetadataValidation(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := m.AddAccountWithInput(AddAccountInput{Name: "主號", ICloudEmail: "a@icloud.com"})
	if err != nil {
		t.Fatal(err)
	}
	id := sum.ID

	if _, err := m.UpdateMetadata(id, UpdateAccountInput{}); err == nil {
		t.Fatal("空更新應當報錯")
	}
	empty := ""
	if _, err := m.UpdateMetadata(id, UpdateAccountInput{Name: &empty}); err == nil {
		t.Fatal("空名稱應當報錯")
	}
	badHost := "evil.com"
	if _, err := m.UpdateMetadata(id, UpdateAccountInput{Host: &badHost}); err == nil {
		t.Fatal("非法主機應當報錯")
	}
	good := "新名稱"
	got, err := m.UpdateMetadata(id, UpdateAccountInput{Name: &good})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "新名稱" {
		t.Fatalf("期望名稱被更新為 %q,得到 %q", "新名稱", got.Name)
	}
	// 編輯不修改憑證
	if got.HasCookies || got.HasAppPassword {
		t.Fatalf("編輯不應產生憑證: %+v", got)
	}
}

// TestUpdateProxyClearsProxy 驗證代理清除。
func TestUpdateProxyClearsProxy(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := m.AddAccountWithInput(AddAccountInput{Name: "主號", ICloudEmail: "a@icloud.com", Proxy: "http://u:p@proxy.example.com:8080"})
	if err != nil {
		t.Fatal(err)
	}
	if !sum.HasProxy {
		t.Fatalf("期望 HasProxy=true: %+v", sum)
	}
	got, err := m.UpdateProxy(sum.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.HasProxy {
		t.Fatalf("期望清除代理: %+v", got)
	}
}

// TestListSummariesStableOrder 驗證 active → pending → error 排序及同狀態穩定排序。
func TestListSummariesStableOrder(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.accounts = map[string]*Account{
		"acc_3": {ID: "acc_3", Name: "錯誤號", Status: "error"},
		"acc_2": {ID: "acc_2", Name: "等待號", Status: "pending"},
		"acc_1": {ID: "acc_1", Name: "活躍號", Status: "active"},
		"acc_4": {ID: "acc_4", Name: "活躍零號", Status: "active"},
	}
	m.mu.Unlock()

	sums := m.ListSummaries()
	var got []string
	for _, s := range sums {
		got = append(got, s.ID)
	}
	want := []string{"acc_1", "acc_4", "acc_2", "acc_3"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("排序錯誤: 得到 %v,期望 %v", got, want)
	}
}

// TestAddAccountWithInputNoNetwork 驗證無 Cookie 的新增不訪問網路。
func TestAddAccountWithInputNoNetwork(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := m.AddAccountWithInput(AddAccountInput{Name: "主號", ICloudEmail: "a@icloud.com"})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Status != "pending" {
		t.Fatalf("無 Cookie 新增期望 pending,得到 %s", sum.Status)
	}
	if sum.ID == "" || !strings.HasPrefix(sum.ID, "acc_") {
		t.Fatalf("ID 格式錯誤: %q", sum.ID)
	}
}

// TestUpdateProxyInvalid 驗證非法代理報固定文案且不洩露 URL。
func TestUpdateProxyInvalid(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := m.AddAccountWithInput(AddAccountInput{Name: "主號", ICloudEmail: "a@icloud.com"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.UpdateProxy(sum.ID, "ftp://user:proxy-secret@host:21")
	if err == nil {
		t.Fatal("非法代理應當報錯")
	}
	if strings.Contains(err.Error(), "proxy-secret") || strings.Contains(err.Error(), "ftp://") {
		t.Fatalf("錯誤不應洩露代理內容: %v", err)
	}
}

// TestNormalizeICloudEmail 驗證"只填 Prefix"與完整信箱兩種輸入。
func TestNormalizeICloudEmail(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		host string
		want string
		bad  bool
	}{
		{"完整信箱原樣保留", "owner@icloud.com", "icloud.com", "owner@icloud.com", false},
		{"完整國區信箱原樣保留", "owner@icloud.com.cn", "icloud.com.cn", "owner@icloud.com.cn", false},
		{"僅 Prefix 補全全球區", "owner", "icloud.com", "owner@icloud.com", false},
		{"僅 Prefix 補全國區", "owner", "icloud.com.cn", "owner@icloud.com.cn", false},
		{"帶點的 Prefix", "john.doe", "icloud.com", "john.doe@icloud.com", false},
		{"host 為空回退預設", "owner", "", "owner@icloud.com", false},
		{"空輸入報錯", "   ", "icloud.com", "", true},
		{"含空白報錯", "bad prefix", "icloud.com", "", true},
		{"帶顯示名的信箱報錯", `"Quoted" <a@icloud.com>`, "icloud.com", "", true},
		{"非法字元報錯", "bad$prefix", "icloud.com", "", true},
	}
	for _, tc := range cases {
		got, err := NormalizeICloudEmail(tc.raw, tc.host)
		if tc.bad {
			if err == nil {
				t.Errorf("%s: 期望報錯,得到 %q", tc.name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: 期望成功,得到 %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: 期望 %q,得到 %q", tc.name, tc.want, got)
		}
	}
}

// TestAddAccountDerivesNameAndEmailFromPrefix 驗證只填 Prefix 時信箱與名稱自動補全。
func TestAddAccountDerivesNameAndEmailFromPrefix(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := m.AddAccountWithInput(AddAccountInput{ICloudEmail: "john.doe", Host: "icloud.com.cn"})
	if err != nil {
		t.Fatal(err)
	}
	if sum.ICloudEmail != "john.doe@icloud.com.cn" {
		t.Fatalf("期望補全國區信箱,得到 %q", sum.ICloudEmail)
	}
	if sum.Name != "john.doe" {
		t.Fatalf("期望名稱由 Prefix 推導為 %q,得到 %q", "john.doe", sum.Name)
	}

	// 顯式名稱優先於推導
	sum2, err := m.AddAccountWithInput(AddAccountInput{Name: "我的主號", ICloudEmail: "other"})
	if err != nil {
		t.Fatal(err)
	}
	if sum2.Name != "我的主號" {
		t.Fatalf("期望使用顯式名稱,得到 %q", sum2.Name)
	}
	if sum2.ICloudEmail != "other@icloud.com" {
		t.Fatalf("期望補全預設區信箱,得到 %q", sum2.ICloudEmail)
	}
}

// TestUpdateMetadataAcceptsPrefix 驗證 PATCH 也接受 Prefix 並用帳號 host 補全。
func TestUpdateMetadataAcceptsPrefix(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := m.AddAccountWithInput(AddAccountInput{
		Name:        "主號",
		ICloudEmail: "a@icloud.com.cn",
		Host:        "icloud.com.cn",
	})
	if err != nil {
		t.Fatal(err)
	}
	prefix := "newname"
	got, err := m.UpdateMetadata(sum.ID, UpdateAccountInput{ICloudEmail: &prefix})
	if err != nil {
		t.Fatal(err)
	}
	if got.ICloudEmail != "newname@icloud.com.cn" {
		t.Fatalf("期望用帳號 host 補全為 newname@icloud.com.cn,得到 %q", got.ICloudEmail)
	}
}
