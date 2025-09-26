package emailparser

import (
	"fmt"
)

func (p *EmailParser) DebugShow() {
	// 激活获取所有字段
	// 输出邮件头信息
	fmt.Printf("Message-ID: %s\n", p.MessageID)
	fmt.Printf("Subject: %s\n", p.Subject)
	fmt.Printf("Date: %s, unix: %d\n", p.Date, p.DateUnix)
	fmt.Printf("From: %s <%s>\n", p.From.Name, p.From.Email)
	fmt.Printf("To:\n")
	for _, to := range p.To {
		fmt.Printf("  %s <%s>; %s\n", to.Name, to.Email, string(to.NameRaw))
	}
	fmt.Printf("Cc:\n")
	for _, cc := range p.Cc {
		fmt.Printf("  %s <%s>\n", cc.Name, cc.Email)
	}
	fmt.Printf("Bcc:\n")
	for _, bcc := range p.Bcc {
		fmt.Printf("  %s <%s>\n", bcc.Name, bcc.Email)
	}
	fmt.Printf("Sender: %s <%s>\n", p.Sender.Name, p.Sender.Email)
	fmt.Printf("Reply-To: %s <%s>\n", p.ReplyTo.Name, p.ReplyTo.Email)
	fmt.Printf("Disposition-Notification-To: %s <%s>\n", p.DispositionNotificationTo.Name, p.DispositionNotificationTo.Email)
	fmt.Printf("References:\n")
	for _, ref := range p.GetReferences() {
		fmt.Printf("  %s\n", ref)
	}

	// 正文
	tmpAlternativeShowNodes := p.GetAlternativeShowNodes()
	alternativeShowNodes := make(map[*MIMENode]bool)
	for _, n := range tmpAlternativeShowNodes {
		alternativeShowNodes[n] = true
	}
	fmt.Printf("Text Nodes:\n")
	for _, n := range p.GetTextNodes() {
		fmt.Printf("---Text Node---\n")
		fmt.Printf("Alternative Show Node: %v\n", alternativeShowNodes[n])
		fmt.Printf("Content-Type: %s\n", n.ContentType)
		fmt.Printf("Encoding: %s\n", n.Encoding)
		fmt.Printf("Charset: %s\n", n.Charset)
		con := n.GetDecodedTextContent()
		fmt.Printf("Size: %d\n", len([]byte(con)))
		if len(con) > 120 {
			con = con[0:120] + "..."
		}
		fmt.Printf("  %s\n", string(con))
	}
	// 附件
	fmt.Printf("Attachment Nodes:\n")
	for _, n := range p.GetAttachmentNodes() {
		fmt.Printf("---Attachment Node---\n")
		fmt.Printf("Content-Type: %s\n", n.ContentType)
		fmt.Printf("Content-ID: %s\n", n.ContentID)
		fmt.Printf("FileName: %s\n", n.Filename)
		fmt.Printf("Name: %s\n", n.Name)
		fmt.Printf("IsInline: %v\n", n.IsInlineAttachment())
		fmt.Printf("IsTnef: %v\n", n.IsTnef("CONTENT-TYPE"))
		fmt.Printf("Size: %d\n", len(n.GetDecodedContent()))
	}
}
