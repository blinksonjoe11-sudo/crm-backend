package mailer

import (
	"bytes"
	"fmt"
	"html/template"

	mailjet "github.com/mailjet/mailjet-apiv3-go/v4"
)

type Mailer struct {
	client    *mailjet.Client
	fromEmail string
	fromName  string
}

type Mail struct {
	To      string
	ToName  string
	Subject string
	Body    string
}

func New(apiKeyPublic, apiKeyPrivate, fromEmail, fromName string) *Mailer {
	client := mailjet.NewMailjetClient(apiKeyPublic, apiKeyPrivate)
	return &Mailer{
		client:    client,
		fromEmail: fromEmail,
		fromName:  fromName,
	}
}

func RenderTemplate(tmplStr string, data interface{}) (string, error) {
	t, err := template.New("email").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("mailer: template parse error: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("mailer: template execute error: %w", err)
	}
	return buf.String(), nil
}

func (m *Mailer) Send(mail Mail) error {
	toName := mail.ToName
	if toName == "" {
		toName = mail.To
	}

	messages := mailjet.MessagesV31{
		Info: []mailjet.InfoMessagesV31{
			{
				From: &mailjet.RecipientV31{
					Email: m.fromEmail,
					Name:  m.fromName,
				},
				To: &mailjet.RecipientsV31{
					{
						Email: mail.To,
						Name:  toName,
					},
				},
				Subject:  mail.Subject,
				HTMLPart: mail.Body,
			},
		},
	}

	_, err := m.client.SendMailV31(&messages)
	if err != nil {
		return fmt.Errorf("mailer: send failed: %w", err)
	}
	return nil
}

// ─── pre-built email bodies ───────────────────────────────────────────────────

const WelcomeTemplate = `
<!DOCTYPE html><html><body style="font-family:Arial,sans-serif;color:#1E3A5F">
<h2>Welcome to CRM, {{.Name}}!</h2>
<p>Your workspace <strong>{{.WorkspaceName}}</strong> is ready.</p>
<p><a href="{{.AppURL}}" style="background:#1A56DB;color:#fff;padding:10px 20px;border-radius:4px;text-decoration:none">Open CRM</a></p>
</body></html>`

const PasswordResetTemplate = `
<!DOCTYPE html><html><body style="font-family:Arial,sans-serif;color:#1E3A5F">
<h2>Reset your password</h2>
<p>Click the link below. It expires in 1 hour.</p>
<p><a href="{{.ResetURL}}" style="background:#1A56DB;color:#fff;padding:10px 20px;border-radius:4px;text-decoration:none">Reset Password</a></p>
<p style="color:#6B7280;font-size:12px">If you didn't request this, ignore this email.</p>
</body></html>`

const InviteTemplate = `
<!DOCTYPE html><html><body style="font-family:Arial,sans-serif;color:#1E3A5F">
<h2>You've been invited to {{.WorkspaceName}}</h2>
<p>{{.InviterName}} has invited you to join their CRM workspace.</p>
<p><a href="{{.InviteURL}}" style="background:#1A56DB;color:#fff;padding:10px 20px;border-radius:4px;text-decoration:none">Accept Invitation</a></p>
</body></html>`
