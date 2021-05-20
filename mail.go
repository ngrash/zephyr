package main

import (
	"fmt"
	"log"
	"net/smtp"
	"strings"
)

type Mailer struct {
	valid                                      bool
	smtpHost, smtpPort, smtpUser, smtpPassword string
	baseURL                                    string
}

func NullMailer() Mailer {
	return Mailer{}
}

func NewMailer(smtpHost, smtpPort, smtpUser, smtpPassword, baseURL string) Mailer {
	const noMail = "Email notifications are disabled."

	if smtpHost == "" {
		log.Println("Missing SMTP host. " + noMail)
		return NullMailer()
	}
	if smtpPort == "" {
		log.Println("Missing SMTP port. " + noMail)
		return NullMailer()
	}

	if smtpUser == "" {
		log.Println("Missing SMTP user. " + noMail)
		return NullMailer()
	}

	if smtpPassword == "" {
		log.Println("Missing SMTP password. " + noMail)
		return NullMailer()
	}

	return Mailer{true, smtpHost, smtpPort, smtpUser, smtpPassword, baseURL}
}

func (m Mailer) Send(to, subject, body string) error {
	if !m.valid {
		return nil
	}

	preparedBody := strings.ReplaceAll(body, "\n", "\r\n")
	preparedBody = strings.ReplaceAll(preparedBody, "__BASE_URL__", m.baseURL)

	auth := smtp.PlainAuth("", m.smtpUser, m.smtpPassword, m.smtpHost)

	msg := []byte("From: " + m.smtpUser + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: [Zephyr] " + subject + "\r\n" +
		"\r\n" +
		preparedBody + "\r\n")

	return smtp.SendMail(fmt.Sprintf("%s:%s", m.smtpHost, m.smtpPort), auth, m.smtpUser, []string{to}, msg)
}
