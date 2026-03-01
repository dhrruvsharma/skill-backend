package utils

import (
	"bytes"
	"fmt"
	"html/template"
	"net/smtp"
	"os"
)

type EmailPayload struct {
	To      string
	Subject string
	Body    string
}

func Send(payload EmailPayload) error {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASS")
	from := os.Getenv("SMTP_FROM")

	auth := smtp.PlainAuth("", user, pass, host)

	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		from, payload.To, payload.Subject, payload.Body,
	)

	return smtp.SendMail(host+":"+port, auth, user, []string{payload.To}, []byte(msg))
}

func SendVerificationEmail(toEmail, firstName, otp string) error {
	const tmpl = `<!DOCTYPE html>
<html>
<body style="font-family:Arial,sans-serif;max-width:600px;margin:auto;padding:24px">
  <h2 style="color:#4F46E5">Verify your email</h2>
  <p>Hi {{.FirstName}},</p>
  <p>Use the OTP below to verify your email address. It expires in <strong>10 minutes</strong>.</p>
  <div style="font-size:36px;font-weight:bold;letter-spacing:12px;color:#4F46E5;margin:32px 0">{{.OTP}}</div>
  <p style="color:#6B7280;font-size:13px">If you didn't create an account, you can safely ignore this email.</p>
</body>
</html>`

	body, err := renderTemplate(tmpl, map[string]string{"FirstName": firstName, "OTP": otp})
	if err != nil {
		return err
	}
	return Send(EmailPayload{To: toEmail, Subject: "Your verification OTP", Body: body})
}

func SendPasswordResetEmail(toEmail, firstName, resetLink string) error {
	const tmpl = `<!DOCTYPE html>
<html>
<body style="font-family:Arial,sans-serif;max-width:600px;margin:auto;padding:24px">
  <h2 style="color:#4F46E5">Reset your password</h2>
  <p>Hi {{.FirstName}},</p>
  <p>Click the button below to reset your password. The link expires in <strong>1 hour</strong>.</p>
  <a href="{{.Link}}" style="display:inline-block;margin:24px 0;padding:12px 24px;background:#4F46E5;color:#fff;border-radius:6px;text-decoration:none;font-weight:bold">
    Reset Password
  </a>
  <p style="color:#6B7280;font-size:13px">If you didn't request a password reset, ignore this email.</p>
</body>
</html>`

	body, err := renderTemplate(tmpl, map[string]string{"FirstName": firstName, "Link": resetLink})
	if err != nil {
		return err
	}
	return Send(EmailPayload{To: toEmail, Subject: "Reset your password", Body: body})
}

func renderTemplate(tmplStr string, data map[string]string) (string, error) {
	t, err := template.New("email").Parse(tmplStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
