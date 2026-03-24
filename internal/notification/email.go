package notification

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/rs/zerolog/log"
	"gopkg.in/gomail.v2"
)

// Emailer sends SMTP notifications via MailHog (or any SMTP server).
type Emailer struct {
	host string
	port int
	from string
}

func NewEmailer(host string, port int, from string) *Emailer {
	return &Emailer{host: host, port: port, from: from}
}

var processedTmpl = template.Must(template.New("processed").Parse(`<!DOCTYPE html>
<html>
<body style="font-family:sans-serif;max-width:600px;margin:40px auto;padding:20px">
  <h2 style="color:#2563eb">File Processing Complete</h2>
  <p>Your file <strong>{{.FileName}}</strong> has been processed successfully.</p>
  <table style="border-collapse:collapse;width:100%">
    <tr><td style="padding:8px;border:1px solid #e5e7eb"><strong>File ID</strong></td>
        <td style="padding:8px;border:1px solid #e5e7eb">{{.FileID}}</td></tr>
    <tr><td style="padding:8px;border:1px solid #e5e7eb"><strong>Status</strong></td>
        <td style="padding:8px;border:1px solid #e5e7eb;color:#16a34a">{{.Status}}</td></tr>
  </table>
  <p style="margin-top:24px;color:#6b7280;font-size:12px">File Upload &amp; Processing Service</p>
</body>
</html>`))

type emailData struct {
	FileName string
	FileID   string
	Status   string
}

// SendProcessed fires an email notification about a processed file.
// Intended to be called as a goroutine — logs errors but never panics.
func (e *Emailer) SendProcessed(to, fileName, fileID, status string) {
	if to == "" {
		return
	}

	var buf bytes.Buffer
	if err := processedTmpl.Execute(&buf, emailData{
		FileName: fileName,
		FileID:   fileID,
		Status:   status,
	}); err != nil {
		log.Error().Err(err).Msg("email template render failed")
		return
	}

	msg := gomail.NewMessage()
	msg.SetHeader("From", e.from)
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", fmt.Sprintf("File processed: %s", fileName))
	msg.SetBody("text/html", buf.String())

	dialer := gomail.NewDialer(e.host, e.port, "", "")
	if err := dialer.DialAndSend(msg); err != nil {
		log.Error().Err(err).Str("to", to).Msg("failed to send email")
		return
	}

	log.Debug().Str("to", to).Str("file_id", fileID).Msg("email sent")
}
