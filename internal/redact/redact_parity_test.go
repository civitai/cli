package redact

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/credscan"
)

// THE PARITY TABLE. Every CJK / i18n and non-ASCII line this PR (#838) was
// audited on, plus the cases where only ONE of two measures fires — byte vs
// character entropy, and a cut quoted value's 512-character fragment vs the
// whole value — with what origin/main did to each: MEASURED on a clean
// origin/main worktree (8591fc4), never computed from the code under test.
//
// Each row is held to "the same as main, or better": main's warning and its
// redacted text exactly, or — for a row main leaked — a warning and a
// redaction now. A row where main redacts and this code does not is a
// regression, and it fails here.
//
// "Warns" is the detector `civitai app submit` runs (credscan.Scan over a
// one-line file); "out" is what `civitai feedback send` would send.

type parityVerdict int

const (
	same     parityVerdict = iota // head must equal main exactly
	better                        // main leaked it; head must warn and redact
	warnsNow                      // main sent the same text but did not warn; head must warn
)

var parityRows = []struct {
	id, line  string
	mainWarns bool
	mainOut   string
	verdict   parityVerdict
	headOut   string
}{
	{"i18n-01", "token_expired: トークンの有効期限（24時間）が切れました",
		false, "token_expired: トークンの有効期限（24時間）が切れました",
		same, "token_expired: トークンの有効期限（24時間）が切れました"},
	{"i18n-02", "\"password_hint\": \"密码长度至少8位，包含字母和数字\",",
		false, "\"password_hint\": \"密码长度至少8位，包含字母和数字\",",
		same, "\"password_hint\": \"密码长度至少8位，包含字母和数字\","},
	{"i18n-03", "secret_note: 秘密鍵は2048ビット以上を推奨します",
		false, "secret_note: 秘密鍵は2048ビット以上を推奨します",
		same, "secret_note: 秘密鍵は2048ビット以上を推奨します"},
	{"i18n-04", "password_policy: รหัสผ่านต้องมีอย่างน้อย8ตัวอักษร",
		false, "password_policy: รหัสผ่านต้องมีอย่างน้อย8ตัวอักษร",
		same, "password_policy: รหัสผ่านต้องมีอย่างน้อย8ตัวอักษร"},
	{"i18n-05", "\"token_invalid\": \"令牌无效，请在30分钟后重试\"",
		false, "\"token_invalid\": \"令牌无效，请在30分钟后重试\"",
		same, "\"token_invalid\": \"令牌无效，请在30分钟后重试\""},
	{"i18n-06", "password_reset: パスワードは12文字以上で設定してください",
		false, "password_reset: パスワードは12文字以上で設定してください",
		same, "password_reset: パスワードは12文字以上で設定してください"},
	{"i18n-07", "api_key_label: APIキー（最大5個まで）",
		true, "api_key_label: <credential>",
		same, "api_key_label: <credential>"},
	{"i18n-08", "\"secret_hint\": \"密钥将在7天后过期\"",
		false, "\"secret_hint\": \"密钥将在7天后过期\"",
		same, "\"secret_hint\": \"密钥将在7天后过期\""},
	{"i18n-09", "token_limit: 每天最多生成100个令牌",
		false, "token_limit: 每天最多生成100个令牌",
		same, "token_limit: 每天最多生成100个令牌"},
	{"i18n-10", "credentials_error: 認証情報が無効です（エラー401）",
		false, "credentials_error: 認証情報が無効です（エラー401）",
		same, "credentials_error: 認証情報が無効です（エラー401）"},
	{"i18n-11", "password_rule: ລະຫັດຜ່ານຕ້ອງມີ8ຕົວ",
		false, "password_rule: ລະຫັດຜ່ານຕ້ອງມີ8ຕົວ",
		same, "password_rule: ລະຫັດຜ່ານຕ້ອງມີ8ຕົວ"},
	{"i18n-12", "\"password_msg\": \"ពាក្យសម្ងាត់ត្រូវមាន8តួ\"",
		false, "\"password_msg\": \"ពាក្យសម្ងាត់ត្រូវមាន8តួ\"",
		same, "\"password_msg\": \"ពាក្យសម្ងាត់ត្រូវមាន8តួ\""},
	{"i18n-13", "access_key_note: 访问密钥ID长度为20位",
		true, "access_key_note: <credential>",
		same, "access_key_note: <credential>"},
	{"i18n-14", "secret_desc: シークレットは64バイト（512ビット）です",
		false, "secret_desc: シークレットは64バイト（512ビット）です",
		same, "secret_desc: シークレットは64バイト（512ビット）です"},
	{"ascii-cjk-1", "登录失败 token=Wq8Zt3Lm6Xr1Bv5，请重试",
		true, "登录失败 token=<credential>",
		same, "登录失败 token=<credential>"},
	{"ascii-cjk-2", "token=Wq8Zt3Lm6Xr1Bv5&lang=中文",
		true, "token=<credential>",
		same, "token=<credential>"},
	{"ascii-cjk-3", "password=Wq8Zt3Lm6Xr1Bv5;备注",
		true, "password=<credential>",
		same, "password=<credential>"},
	{"ascii-cjk-4", "password: Wq8Zt3Lm6Xr1Bv5|用户",
		true, "password: <credential>",
		same, "password: <credential>"},
	{"mixed-1", "DB_PASSWORD=Kq9Zx2Lm7Rb5Tn8Vc中",
		true, "DB_PASSWORD=<credential>",
		same, "DB_PASSWORD=<credential>"},
	{"mixed-2", "password=Wq8中Zt3Lm6Xr1Bv5Tn8Vc",
		true, "password=<credential>",
		same, "password=<credential>"},
	{"mixed-3", "DB_PASSWORD=秘密Wq8Zt3Lm6Xr1Bv5",
		true, "DB_PASSWORD=<credential>",
		same, "DB_PASSWORD=<credential>"},
	{"cyrillic", "DB_PASSWORD=Пароль2024Секрет",
		false, "DB_PASSWORD=Пароль2024Секрет",
		same, "DB_PASSWORD=Пароль2024Секрет"},
	{"arabic-indic", "DB_PASSWORD=KqZxLmRbTnVc٢٠٢٤",
		false, "DB_PASSWORD=KqZxLmRbTnVc٢٠٢٤",
		same, "DB_PASSWORD=KqZxLmRbTnVc٢٠٢٤"},
	{"byte-entropy", "DB_PASSWORD=a1ЖЩЯФЫВАПРО",
		false, "DB_PASSWORD=a1ЖЩЯФЫВАПРО",
		better, "DB_PASSWORD=<credential>"},
	{"byte-entropy-only", "DB_PASSWORD=Kq9中中中中中密密密密密钥钥钥钥钥",
		true, "DB_PASSWORD=<credential>",
		same, "DB_PASSWORD=<credential>"},
	{"fragment-only-reject", `{"token": "` + paritySecret(520, 0) + `test"}`,
		true, "{\"token\": \"<credential>\"}",
		same, "{\"token\": \"<credential>\"}"},
	{"fragment-only-interp", `{"token": "` + paritySecret(520, 0) + `${HOME}"}`,
		true, "{\"token\": \"<credential>\"}",
		same, "{\"token\": \"<credential>\"}"},
	{"fragment-only-entropy", `{"token": "` + paritySecret(520, 0) + strings.Repeat("a", 3000) + `"}`,
		true, "{\"token\": \"<credential>\"}",
		same, "{\"token\": \"<credential>\"}"},
	{"url-byte-entropy-only", "dial redis://cache_rw:密密密中中中钥钥@cache-3.fieldstone.dev:6379/0 timed out",
		true, "dial redis://<credential>@cache-3.fieldstone.dev:6379/0 timed out",
		same, "dial redis://<credential>@cache-3.fieldstone.dev:6379/0 timed out"},
	{"url-char-entropy-only", "dial redis://cache_rw:ЖЩЯФЫВА@cache-3.fieldstone.dev:6379/0 timed out",
		false, "dial redis://<credential>@cache-3.fieldstone.dev:6379/0 timed out",
		warnsNow, "dial redis://<credential>@cache-3.fieldstone.dev:6379/0 timed out"},
	{"f1-quoted-k1", `PASSWORD="` + strings.Repeat("中", 1) + paritySecret(508, 0) + ` second part of value"`,
		true, "PASSWORD=\"<credential>\"",
		same, "PASSWORD=\"<credential>\""},
	{"f1-json-k1", `{"token": "` + strings.Repeat("中", 1) + paritySecret(508, 0) + ` second part of value", "x": 1}`,
		true, "{\"token\": \"<credential>\", \"x\": 1}",
		same, "{\"token\": \"<credential>\", \"x\": 1}"},
	{"f1-unquoted-k1", "SECRET=" + strings.Repeat("中", 1) + paritySecret(509, 0) + "\fMoreSecretStuff9x",
		true, "SECRET=<credential>",
		same, "SECRET=<credential>"},
	{"f1-quoted-k6", `PASSWORD="` + strings.Repeat("中", 6) + paritySecret(493, 0) + ` second part of value"`,
		true, "PASSWORD=\"<credential>\"",
		same, "PASSWORD=\"<credential>\""},
	{"f1-json-k6", `{"token": "` + strings.Repeat("中", 6) + paritySecret(493, 0) + ` second part of value", "x": 1}`,
		true, "{\"token\": \"<credential>\", \"x\": 1}",
		same, "{\"token\": \"<credential>\", \"x\": 1}"},
	{"f1-unquoted-k6", "SECRET=" + strings.Repeat("中", 6) + paritySecret(494, 0) + "\fMoreSecretStuff9x",
		true, "SECRET=<credential>",
		same, "SECRET=<credential>"},
	{"whole-only-escaped-json", `{"credentials":"{\"client_id\":\"abc\",\"client_secret\":\"` + paritySecret(600, 11) + `\"}"}`,
		false, `{"credentials":"{\"client_id\":\"abc\",\"client_secret\":\"` + paritySecret(600, 11) + `\"}"}`,
		better, "{\"credentials\":\"<credential>\"}"},
}

// paritySecret returns n characters of a fixed high-entropy alphabet starting
// seed steps in — the same S520 / S600 the measurement probe used.
func paritySecret(n, seed int) string {
	const alphabet = "Kq9Zx2Lm7Rb5Tn8Vc4Hd6Yw1Pf3Sg0Uj"
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteByte(alphabet[((i+seed)*7)%len(alphabet)])
	}
	return b.String()
}

func TestParityWithMain(t *testing.T) {
	dir := t.TempDir()
	for i, r := range parityRows {
		t.Run(r.id, func(t *testing.T) {
			name := fmt.Sprintf("row%02d.env", i)
			if err := os.WriteFile(filepath.Join(dir, name), []byte(r.line+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			rep := credscan.Scan(dir, []string{name})
			if rep.FilesScanned != 1 {
				t.Fatalf("CONTROL failure: Scan read %d of 1 file, so its verdict says nothing", rep.FilesScanned)
			}
			warns := len(rep.Findings) > 0
			out := New(Options{}).Apply(r.line)

			switch r.verdict {
			case same:
				if warns != r.mainWarns {
					t.Errorf("warns=%v, main warns=%v", warns, r.mainWarns)
				}
				if out != r.mainOut {
					t.Errorf("sends %q\n main sent %q", out, r.mainOut)
				}
			case warnsNow:
				if r.mainWarns {
					t.Fatalf("row is marked warnsNow, but main already warned")
				}
				if !warns || out != r.mainOut {
					t.Errorf("warns=%v and sends %q; want a warning and main's %q", warns, out, r.mainOut)
				}
			case better:
				if r.mainWarns || r.mainOut != r.line {
					t.Fatalf("row is marked better, but main did not leak it (warns=%v, out=%q)", r.mainWarns, r.mainOut)
				}
				if !warns || out != r.headOut {
					t.Errorf("warns=%v and sends %q; want a warning and %q", warns, out, r.headOut)
				}
			}
		})
	}
	// CONTROL: the table exercises both outcomes, so "same as main" is not a
	// fact about one kind of row.
	warned, quiet := 0, 0
	for _, r := range parityRows {
		if r.mainWarns {
			warned++
		} else {
			quiet++
		}
	}
	if warned == 0 || quiet == 0 {
		t.Fatalf("CONTROL failure: main warned on %d row(s) and stayed quiet on %d", warned, quiet)
	}
}
