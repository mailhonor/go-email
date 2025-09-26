package emailparser

import (
	"fmt"
)

func (p *EmailParser) DebugShow() {
	// 激活获取所有字段
	p.GetMessageID()
	p.GetSubject()
	p.GetDate()
	p.GetDateUnix()
	p.GetFrom()
	p.GetTo()
	p.GetCc()
	p.GetBcc()
	p.GetSender()
	p.GetReplyTo()
	p.GetDispositionNotificationTo()
	// 输出邮件头信息
	fmt.Printf("Message-ID: %s\n", p.messageID)
	fmt.Printf("Subject: %s\n", p.subject)
	fmt.Printf("Date: %s, unix: %d\n", p.date, p.dateUnix)
	fmt.Printf("From: %s <%s>\n", p.from.Name, p.from.Email)
	fmt.Printf("To:\n")
	for _, to := range p.to {
		fmt.Printf("  %s <%s>; %s\n", to.Name, to.Email, string(to.NameRaw))
	}
	fmt.Printf("Cc:\n")
	for _, cc := range p.cc {
		fmt.Printf("  %s <%s>\n", cc.Name, cc.Email)
	}
	fmt.Printf("Bcc:\n")
	for _, bcc := range p.bcc {
		fmt.Printf("  %s <%s>\n", bcc.Name, bcc.Email)
	}
	fmt.Printf("Sender: %s <%s>\n", p.sender.Name, p.sender.Email)
	fmt.Printf("Reply-To: %s <%s>\n", p.replyTo.Name, p.replyTo.Email)
	fmt.Printf("Disposition-Notification-To: %s <%s>\n", p.dispositionNotificationTo.Name, p.dispositionNotificationTo.Email)
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
		con := GetDecodedTextContent(n)
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
		fmt.Printf("IsInline: %v\n", IsInlineAttachment(n))
		fmt.Printf("IsTnef: %v\n", n.IsTnef("CONTENT-TYPE"))
		fmt.Printf("Size: %d\n", len(GetDecodedContent(n)))
	}
}
