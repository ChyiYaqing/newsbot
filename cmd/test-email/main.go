// test-email sends a test email using the SMTP config from .env / environment variables.
// Usage: go run ./cmd/test-email <recipient>
package main

import (
	"fmt"
	"os"

	"github.com/chyiyaqing/newsbot/internal/config"
	"github.com/chyiyaqing/newsbot/internal/notify/email"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: go run ./cmd/test-email <recipient-email>\n")
		os.Exit(1)
	}
	to := os.Args[1]

	cfg, err := config.Load("newsbot.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	c := email.New(cfg.SMTP.Host, cfg.SMTP.Port, cfg.SMTP.Username, cfg.SMTP.Password, cfg.SMTP.From, cfg.SMTP.SiteURL)
	if c == nil {
		fmt.Fprintln(os.Stderr, "SMTP not configured (SMTP_HOST and SMTP_FROM are required)")
		os.Exit(1)
	}

	fmt.Printf("Sending test email to %s via %s:%d ...\n", to, cfg.SMTP.Host, cfg.SMTP.Port)

	body := `<!DOCTYPE html><html><body style="font-family:sans-serif;padding:20px">
<h2>NewsBot 邮件测试</h2>
<p>如果您收到这封邮件，说明 SMTP 配置正确。</p>
<ul>
  <li>Host: ` + cfg.SMTP.Host + `</li>
  <li>From: ` + cfg.SMTP.From + `</li>
  <li>Site: ` + cfg.SMTP.SiteURL + `</li>
</ul>
</body></html>`

	if err := c.SendHTML(to, "NewsBot 邮件测试", body); err != nil {
		fmt.Fprintf(os.Stderr, "FAILED: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("OK: email sent successfully")
}
