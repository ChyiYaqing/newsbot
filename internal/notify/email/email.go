package email

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/mail"
	"net/smtp"
	"strings"

	"github.com/chyiyaqing/newsbot/internal/ai"
	"github.com/chyiyaqing/newsbot/internal/store"
)

// Client sends HTML emails via SMTP.
type Client struct {
	host     string
	port     int
	username string
	password string
	from     string
	siteURL  string
}

// New creates an email client. Returns nil if host or from is empty.
func New(host string, port int, username, password, from, siteURL string) *Client {
	if host == "" || from == "" {
		return nil
	}
	if port == 0 {
		port = 587
	}
	return &Client{host: host, port: port, username: username, password: password, from: from, siteURL: siteURL}
}

// GenerateToken generates a random unsubscribe token.
func GenerateToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// SendHTML sends an HTML email to a single recipient.
func (c *Client) SendHTML(to, subject, htmlBody string) error {
	msg := buildMessage(c.from, to, subject, htmlBody)
	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	fromAddr := extractAddress(c.from)

	if c.port == 465 {
		return c.sendTLS(addr, fromAddr, to, msg)
	}

	var auth smtp.Auth
	if c.username != "" {
		auth = smtp.PlainAuth("", c.username, c.password, c.host)
	}
	return smtp.SendMail(addr, auth, fromAddr, []string{to}, msg)
}

// extractAddress returns just the email address from a "Name <addr>" string.
func extractAddress(from string) string {
	if addr, err := mail.ParseAddress(from); err == nil {
		return addr.Address
	}
	return from
}

func (c *Client) sendTLS(addr, from, to string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: c.host})
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	client, err := smtp.NewClient(conn, c.host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Quit() //nolint:errcheck

	if c.username != "" {
		auth := smtp.PlainAuth("", c.username, c.password, c.host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	return w.Close()
}

func buildMessage(from, to, subject, htmlBody string) []byte {
	var sb strings.Builder
	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + to + "\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(htmlBody)
	return []byte(sb.String())
}

// SendWelcome sends a welcome / subscription-confirmation email.
func (c *Client) SendWelcome(to, token string) error {
	subject := "欢迎订阅 NewsBot 技术资讯"
	var unsubURL string
	if c.siteURL != "" && token != "" {
		unsubURL = strings.TrimRight(c.siteURL, "/") + "/api/unsubscribe?token=" + token
	}

	var sb strings.Builder
	sb.WriteString(`<!DOCTYPE html><html><head><meta charset="UTF-8"></head>`)
	sb.WriteString(`<body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;max-width:640px;margin:0 auto;padding:40px 20px;color:#0f172a;line-height:1.6">`)
	sb.WriteString(`<h1 style="font-size:20px;margin-bottom:8px">欢迎订阅 NewsBot！</h1>`)
	sb.WriteString(`<p style="color:#475569">您已成功订阅 NewsBot 技术资讯。我们会定期为您推送精选技术文章与趋势分析。</p>`)
	if unsubURL != "" {
		sb.WriteString(fmt.Sprintf(
			`<p style="margin-top:32px;font-size:11px;color:#94a3b8;border-top:1px solid #e2e8f0;padding-top:16px">不想再收到邮件？<a href="%s" style="color:#94a3b8">取消订阅</a></p>`,
			escapeAttr(unsubURL),
		))
	}
	sb.WriteString(`</body></html>`)

	return c.SendHTML(to, subject, sb.String())
}

// FormatEmailReport builds an HTML email body from analyzed articles and trends.
func FormatEmailReport(articles []store.ArticleWithAnalysis, trends *ai.TrendReport, window, unsubscribeToken, siteURL string) string {
	var sb strings.Builder

	sb.WriteString(`<!DOCTYPE html><html><head><meta charset="UTF-8"></head>`)
	sb.WriteString(`<body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;max-width:640px;margin:0 auto;padding:20px;color:#0f172a;line-height:1.6">`)

	sb.WriteString(fmt.Sprintf(`<h1 style="font-size:20px;margin-bottom:4px">NewsBot — %d 篇新文章</h1>`, len(articles)))
	sb.WriteString(fmt.Sprintf(`<p style="color:#94a3b8;font-size:13px;margin:0 0 24px">过去 %s 的技术动态</p>`, windowLabel(window)))

	limit := 20
	if len(articles) < limit {
		limit = len(articles)
	}

	if limit > 0 {
		sb.WriteString(`<h2 style="font-size:15px;font-weight:600;border-bottom:1px solid #e2e8f0;padding-bottom:8px;margin-bottom:16px">Top Articles</h2>`)
		sb.WriteString(`<ol style="padding-left:20px;margin:0">`)
		for i := 0; i < limit; i++ {
			a := articles[i]
			sb.WriteString(`<li style="margin-bottom:16px">`)
			sb.WriteString(fmt.Sprintf(`<a href="%s" style="font-size:15px;font-weight:600;color:#0f172a;text-decoration:none">%s</a>`,
				escapeAttr(a.Article.URL), escapeHTML(a.Article.Title)))
			if a.ArticleAnalysis.TitleCN != "" {
				sb.WriteString(fmt.Sprintf(`<br><span style="font-size:13px;color:#475569">%s</span>`, escapeHTML(a.ArticleAnalysis.TitleCN)))
			}
			if a.ArticleAnalysis.RecommendReason != "" {
				sb.WriteString(fmt.Sprintf(`<br><span style="font-size:12px;color:#64748b">%s</span>`, escapeHTML(a.ArticleAnalysis.RecommendReason)))
			}
			sb.WriteString(fmt.Sprintf(`<br><span style="font-size:11px;color:#94a3b8">评分: %d | %s | %s</span>`,
				a.ArticleAnalysis.TotalScore, escapeHTML(a.ArticleAnalysis.Category), escapeHTML(a.Article.BlogDomain)))
			sb.WriteString(`</li>`)
		}
		sb.WriteString(`</ol>`)
	}

	if trends != nil && len(trends.Trends) > 0 {
		sb.WriteString(`<h2 style="font-size:15px;font-weight:600;border-bottom:1px solid #e2e8f0;padding-bottom:8px;margin:28px 0 16px">技术趋势</h2>`)
		sb.WriteString(`<ol style="padding-left:20px;margin:0">`)
		for _, t := range trends.Trends {
			sb.WriteString(`<li style="margin-bottom:12px">`)
			sb.WriteString(fmt.Sprintf(`<strong>%s</strong><br>`, escapeHTML(t.Title)))
			sb.WriteString(fmt.Sprintf(`<span style="font-size:13px;color:#475569">%s</span>`, escapeHTML(t.Description)))
			sb.WriteString(`</li>`)
		}
		sb.WriteString(`</ol>`)
	}

	if unsubscribeToken != "" && siteURL != "" {
		unsubURL := strings.TrimRight(siteURL, "/") + "/api/unsubscribe?token=" + unsubscribeToken
		sb.WriteString(fmt.Sprintf(
			`<p style="margin-top:32px;font-size:11px;color:#94a3b8;border-top:1px solid #e2e8f0;padding-top:16px">不想再收到邮件？<a href="%s" style="color:#94a3b8">取消订阅</a></p>`,
			escapeAttr(unsubURL),
		))
	}

	sb.WriteString(`</body></html>`)
	return sb.String()
}

func windowLabel(w string) string {
	switch w {
	case "24h":
		return "24小时"
	case "3days":
		return "3天"
	case "7days":
		return "7天"
	}
	return w
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func escapeAttr(s string) string {
	s = escapeHTML(s)
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}
