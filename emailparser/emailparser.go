package emailparser

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/mail"
	"sort"
	"strings"

	mailhonorcharsetutils "github.com/mailhonor/go-utils/charset"
	mailhonorquotedprintableutils "github.com/mailhonor/go-utils/quotedprintable"
	mailhonorstringutils "github.com/mailhonor/go-utils/strings"
)

type MimeValueTokenNode struct {
	Data    []byte
	Charset string
}

type MimeValueParams struct {
	Value  []byte
	Params map[string][]byte
}

type MimeLine struct {
	Name  string
	Value []byte
}

type MimeAddress struct {
	NameRaw []byte
	Name    string
	Email   string
}

// MIMENode 表示MIME结构中的一个节点（可能是叶子节点或多部分父节点）
type MIMENode struct {
	HeaderStart int // 头部在原始数据中的起始偏移
	HeaderLen   int // 头部长度
	BodyStart   int // 正文在原始数据中的起始偏移
	BodyLen     int // 正文长度

	Header []MimeLine // 解析后的头部键值对

	ContentType string // 媒体类型（如text/plain、multipart/mixed）
	Encoding    string // 传输编码（BASE64/QUOTED-PRINTABLE/空）
	Charset     string // 字符集（如UTF-8、GBK）
	Boundary    string // 多部分分隔符（仅multipart类型有效）
	Filename    string // 附件文件名
	Name        string // 附件文件名
	ContentID   string // 内容ID（用于内嵌资源）
	Disposition string // 内容处置（如INLINE/ATTACHMENT）
	isTnef      bool   // 是否为TNEF编码（仅APPLICATION/MS-TNEF类型有效）
	isInline    bool   // 是否为内嵌附件

	//
	EmailParser *EmailParser
	Parent      *MIMENode   // 父节点
	Childs      []*MIMENode // 子节点
}

type EmailParserOptions struct {
	DefaultCharset string // 默认字符集（如UTF-8、GBK）
	EmailData      []byte // 原始邮件数据
}

type EmailParser struct {
	DefaultCharset                  string
	EmailData                       []byte
	topNode                         *MIMENode
	messageID                       string
	messageIDDealed                 bool
	subject                         string
	subjectDealed                   bool
	date                            string
	dateUnix                        int64
	dateDealed                      bool
	from                            MimeAddress
	fromDealed                      bool
	to                              []MimeAddress
	toDealed                        bool
	cc                              []MimeAddress
	ccDealed                        bool
	bcc                             []MimeAddress
	bccDealed                       bool
	sender                          MimeAddress
	senderDealed                    bool
	replyTo                         MimeAddress
	replyToDealed                   bool
	dispositionNotificationTo       MimeAddress
	dispositionNotificationToDealed bool
	references                      []string
	referencesDealed                bool
	textNodes                       []*MIMENode
	attachmentNodes                 []*MIMENode
	nodeClassified                  bool
	alternativeShowNodes            []*MIMENode
	alternativeShowNodesDealed      bool
	inlineAttachmentNodesDealed     bool
}

// boundaryPos 记录一个边界符的位置信息
type boundaryPos struct {
	Boundary string // 边界符内容
	Start    int    // 边界符在原始数据中的起始偏移
	End      int    // 边界符在原始数据中的结束偏移
}

// scanAllBoundaries 全局扫描原始邮件数据，提取所有可能的MIME边界符
func scanAllBoundaries(raw []byte) []boundaryPos {
	var boundaries []boundaryPos
	offset := 0
	rawLen := len(raw)

	for offset < rawLen {
		// 查找边界符起始标记："\n--"（或行首 "--"）
		var start int
		if bytes.HasPrefix(raw[offset:], []byte("--")) {
			start = offset // 行首直接以 "--" 开头
		} else {
			// 查找 "\n--"（换行后的边界符）
			nlPos := bytes.Index(raw[offset:], []byte("\n--"))
			if nlPos == -1 {
				break // 没有更多边界符
			}
			start = offset + nlPos + 1 // 跳过 "\n"，指向 "--" 起始位置
		}

		// 提取边界符内容（从 "--" 后到下一个换行符）
		boundaryStart := start + 2 // 跳过 "--"
		nlPos := bytes.Index(raw[boundaryStart:], []byte("\n"))
		if nlPos == -1 || (boundaryStart+nlPos) > rawLen {
			break // 边界符不完整，终止扫描
		}
		boundaryEnd := boundaryStart + nlPos
		boundaryStr := strings.TrimSpace(string(raw[boundaryStart:boundaryEnd]))

		// 记录边界符（去掉结束标记 "--"）
		boundaries = append(boundaries, boundaryPos{
			Boundary: boundaryStr,
			Start:    start,
			End:      boundaryEnd + 1, // 包含换行符
		})

		// 推进偏移量，继续扫描下一个边界符
		offset = boundaryEnd + 1
	}
	return boundaries
}

func emailParserAppendOneLine(lines *[]MimeLine, lineData []byte) {
	pos := bytes.Index(lineData, []byte(":"))
	if pos == -1 {
		*lines = append(*lines, MimeLine{
			Name: strings.ToUpper(strings.TrimSpace(string(lineData))),
		})
		return
	}
	name := strings.ToUpper(strings.TrimSpace(string(lineData[:pos])))
	value := bytes.TrimSpace(lineData[pos+1:])
	*lines = append(*lines, MimeLine{
		Name:  name,
		Value: value,
	})
}

func (n *MIMENode) GetHeaderValue(headerName string) ([]byte, error) {
	headerName = strings.ToUpper(headerName)
	for _, line := range n.Header {
		if line.Name == headerName {
			return line.Value, nil
		}
	}
	return []byte{}, fmt.Errorf("header(%s) not found", headerName)
}

func (n *MIMENode) GetHeaderValueIgnoreNotFound(headerName string) []byte {
	headerName = strings.ToUpper(headerName)
	for _, line := range n.Header {
		if line.Name == headerName {
			return line.Value
		}
	}
	return []byte{}
}

func (n *MIMENode) IsTnef(headerName string) bool {
	n.EmailParser.classifyNodes()
	return n.isTnef
}

func IsInlineAttachment(n *MIMENode) bool {
	n.EmailParser.classifyInlineAttachmentNodes()
	return n.isInline
}

func GetDecodedContent(n *MIMENode) []byte {
	parser := n.EmailParser
	if n.Encoding == "BASE64" {
		decodedData, err := base64.StdEncoding.DecodeString(string(parser.EmailData[n.BodyStart : n.BodyStart+n.BodyLen]))
		if err != nil {
			return []byte{}
		}
		return decodedData
	} else if n.Encoding == "QUOTED-PRINTABLE" {
		return mailhonorquotedprintableutils.DecodeMimeBody(parser.EmailData[n.BodyStart : n.BodyStart+n.BodyLen])
	} else {
		return parser.EmailData[n.BodyStart : n.BodyStart+n.BodyLen]
	}
}

// text cotnent
func GetDecodedTextContent(n *MIMENode) string {
	data := GetDecodedContent(n)
	return mailhonorcharsetutils.ConvertToUTF8(data, n.Charset, n.EmailParser.DefaultCharset)
}

func (p *EmailParser) parseMimeSelf(emailPartData []byte) *MIMENode {
	node := &MIMENode{
		EmailParser: p,
	}

	// 解析邮件头行
	data := emailPartData
	var logicLine []byte
	for len(data) > 0 {
		idx := bytes.Index(data, []byte("\n"))
		var line []byte
		if idx == -1 {
			line = data
			data = []byte{}
		} else {
			line = data[:idx+1]
			data = data[idx+1:]
		}
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			logicLine = append(logicLine, line[1:]...)
		} else {
			if len(logicLine) > 0 {
				emailParserAppendOneLine(&node.Header, logicLine)
			}
			logicLine = line
		}
		logicLine = mailhonorstringutils.TrimRightBytes(logicLine, []byte("\r\n"))
		if idx != -1 {
			if idx == 0 {
				break
			}
			if idx == 1 && len(line) > 0 && line[0] == '\r' {
				break
			}
		}
	}
	if len(logicLine) > 0 {
		emailParserAppendOneLine(&node.Header, logicLine)
	}
	node.HeaderLen = len(emailPartData) - len(data)
	if node.HeaderLen > 0 && emailPartData[node.HeaderLen-1] == '\n' {
		node.HeaderLen--
	}
	if node.HeaderLen > 0 && emailPartData[node.HeaderLen-1] == '\r' {
		node.HeaderLen--
	}
	node.BodyStart = len(emailPartData) - len(data)
	node.BodyLen = len(data)

	// 解析 CONTENT-TRANSFER-ENCODING
	value, err := node.GetHeaderValue("CONTENT-TRANSFER-ENCODING")
	if err == nil {
		vp := ParseMimeValueParams(value)
		node.Encoding = strings.ToUpper(strings.TrimSpace(string(vp.Value)))
	}
	// 解析	 CONTENT-TYPE
	value, err = node.GetHeaderValue("CONTENT-TYPE")
	if err == nil {
		vp := ParseMimeValueParams(value)
		node.ContentType = strings.ToUpper(strings.TrimSpace(string(vp.Value)))
		node.Charset = strings.ToUpper(string(vp.TrimmedParam("CHARSET")))
		node.Name = vp.ParseParamStringValue("NAME", p.DefaultCharset)
		node.Boundary = string(vp.TrimmedParam("BOUNDARY"))
	}
	if node.ContentType == "" || node.ContentType == "TEXT" {
		node.ContentType = "TEXT/PLAIN"
	}
	// 解析 CONTENT-DISPOSITION
	value, err = node.GetHeaderValue("CONTENT-DISPOSITION")
	if err == nil {
		vp := ParseMimeValueParams(value)
		node.Disposition = strings.ToUpper(strings.TrimSpace(string(vp.Value)))
		node.Filename = vp.ParseParamStringValue("FILENAME", p.DefaultCharset)
	}
	// 解析 CONTENT-ID
	value, err = node.GetHeaderValue("CONTENT-ID")
	if err == nil {
		node.ContentID = string(mailhonorstringutils.TrimBytes(value, []byte("\"<>\r\n\t ")))
	}

	return node
}

func (p *EmailParser) parseMime(offset int, emailPartData []byte, boundaries []boundaryPos) *MIMENode {
	node := p.parseMimeSelf(emailPartData)
	node.HeaderStart += offset
	node.BodyStart += offset

	// 不是MULTIPART类型，直接返回
	if !strings.HasPrefix(node.ContentType, "MULTIPART/") {
		return node
	}

	// 是MULTIPART类型，继续解析子节点
	if node.Boundary == "" {
		return node
	}

	//
	lastId := -1
	for i := 0; i < len(boundaries); i++ {
		bo := boundaries[i]
		if bo.Boundary != node.Boundary && bo.Boundary != node.Boundary+"--" {
			continue
		}
		if lastId == -1 {
			lastId = i
			continue
		}
		so := boundaries[lastId]
		eo := boundaries[i]
		var newBoundaries []boundaryPos
		if lastId+1 < i-1 {
			newBoundaries = boundaries[lastId+1 : i-1]
		}
		newNode := p.parseMime(so.End, p.EmailData[so.End:eo.Start-1], newBoundaries)
		newNode.Parent = node
		node.Childs = append(node.Childs, newNode)
		lastId = i
	}
	if lastId > -1 {
		so := boundaries[lastId]
		tmpdata := p.EmailData[so.End:]
		if len(tmpdata) > 0 {
			tmpdata = mailhonorstringutils.TrimBytes(tmpdata, []byte("\r\n\t "))
			if len(tmpdata) > 10 && bytes.Contains(tmpdata, []byte("\n")) {
				newNode := p.parseMime(so.End, tmpdata, []boundaryPos{})
				newNode.Parent = node
				node.Childs = append(node.Childs, newNode)
			}
		}
	}
	return node
}

func (p *EmailParser) parseDate() {
	date, err := p.topNode.GetHeaderValue("DATE")
	if err == nil {
		p.date = strings.TrimSpace(string(date))
	} else {
		received, err := p.topNode.GetHeaderValue("RECEIVED")
		if err == nil {
			s := string(received)
			pos := strings.LastIndex(s, ";")
			if pos > 0 {
				p.date = strings.TrimSpace(s[pos+1:])
			}
		}
	}
	if p.date != "" {
		t, err := mail.ParseDate(p.date)
		if err == nil {
			p.dateUnix = t.Unix()
		}
	}
}

func EmailParserNew(options EmailParserOptions) *EmailParser {
	parser := &EmailParser{
		DefaultCharset: options.DefaultCharset,
		EmailData:      options.EmailData,
	}
	if parser.DefaultCharset == "" {
		parser.DefaultCharset = "UTF-8"
	}
	//
	boundaries := scanAllBoundaries(parser.EmailData)
	parser.topNode = parser.parseMime(0, parser.EmailData, boundaries)
	//
	return parser
}

func (p *EmailParser) GetTopMIMENode() *MIMENode {
	return p.topNode
}

func (p *EmailParser) GetMessageID() string {
	if p.messageIDDealed {
		return p.messageID
	}
	p.messageID = string(mailhonorstringutils.TrimBytes(p.topNode.GetHeaderValueIgnoreNotFound("MESSAGE-ID"), []byte("\"<>\r\n\t ")))
	return p.messageID
}

func (p *EmailParser) GetSubject() string {
	if p.subjectDealed {
		return p.subject
	}
	p.subjectDealed = true
	p.subject = ParseMimeValueString(p.topNode.GetHeaderValueIgnoreNotFound("SUBJECT"), p.DefaultCharset)
	return p.subject
}

func (p *EmailParser) GetDate() string {
	if p.dateDealed {
		return p.date
	}
	p.dateDealed = true
	p.parseDate()
	return p.date
}

func (p *EmailParser) GetDateUnix() int64 {
	if p.dateDealed {
		return p.dateUnix
	}
	p.dateDealed = true
	p.parseDate()
	return p.dateUnix
}

func (p *EmailParser) GetFrom() MimeAddress {
	if p.fromDealed {
		return p.from
	}
	p.fromDealed = true
	p.from = ParseMimeAddressFirstOne(p.topNode.GetHeaderValueIgnoreNotFound("FROM"), p.DefaultCharset)
	return p.from
}

func (p *EmailParser) GetTo() []MimeAddress {
	if p.toDealed {
		return p.to
	}
	p.toDealed = true
	p.to = ParseMimeAddress(p.topNode.GetHeaderValueIgnoreNotFound("TO"), p.DefaultCharset)
	return p.to
}

func (p *EmailParser) GetCc() []MimeAddress {
	if p.ccDealed {
		return p.cc
	}
	p.ccDealed = true
	p.cc = ParseMimeAddress(p.topNode.GetHeaderValueIgnoreNotFound("CC"), p.DefaultCharset)
	return p.cc
}

func (p *EmailParser) GetBcc() []MimeAddress {
	if p.bccDealed {
		return p.bcc
	}
	p.bccDealed = true
	p.bcc = ParseMimeAddress(p.topNode.GetHeaderValueIgnoreNotFound("BCC"), p.DefaultCharset)
	return p.bcc
}

func (p *EmailParser) GetSender() MimeAddress {
	if p.senderDealed {
		return p.sender
	}
	p.senderDealed = true
	p.sender = ParseMimeAddressFirstOne(p.topNode.GetHeaderValueIgnoreNotFound("SENDER"), p.DefaultCharset)
	return p.sender
}

func (p *EmailParser) GetReplyTo() MimeAddress {
	if p.replyToDealed {
		return p.replyTo
	}
	p.replyToDealed = true
	p.replyTo = ParseMimeAddressFirstOne(p.topNode.GetHeaderValueIgnoreNotFound("REPLY-TO"), p.DefaultCharset)
	return p.replyTo
}

func (p *EmailParser) GetDispositionNotificationTo() MimeAddress {
	if p.dispositionNotificationToDealed {
		return p.dispositionNotificationTo
	}
	p.dispositionNotificationToDealed = true
	p.dispositionNotificationTo = ParseMimeAddressFirstOne(p.topNode.GetHeaderValueIgnoreNotFound("DISPOSITION-NOTIFICATION-TO"), p.DefaultCharset)
	return p.dispositionNotificationTo
}

func (p *EmailParser) GetReferences() []string {
	if p.referencesDealed {
		return p.references
	}
	p.referencesDealed = true
	referencesHeader := string(p.topNode.GetHeaderValueIgnoreNotFound("REFERENCES"))
	references := []string{}
	for _, ref := range strings.FieldsFunc(referencesHeader, func(r rune) bool {
		return r == ',' || r == ';' || r == '<' || r == '>' || r == '\t' || r == ' ' || r == '\n' || r == '\r'
	}) {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		references = append(references, ref)
	}
	p.references = references
	return p.references
}

func (p *EmailParser) classifyNodes() {
	if p.nodeClassified {
		return
	}
	p.nodeClassified = true

	var walkAllNode func(node *MIMENode)
	walkAllNode = func(node *MIMENode) {
		typeStr := node.ContentType
		mainType := ""
		if idx := strings.Index(typeStr, "/"); idx > 0 {
			mainType = typeStr[:idx]
		} else {
			mainType = typeStr
		}

		switch mainType {
		case "MULTIPART":
			for _, child := range node.Childs {
				walkAllNode(child)
			}
			return
		case "APPLICATION":
			p.attachmentNodes = append(p.attachmentNodes, node)
			if strings.Contains(typeStr, "TNEF") {
				node.isTnef = true
			}
			return
		case "MESSAGE":
			if strings.Contains(typeStr, "DELIVERY") || strings.Contains(typeStr, "NOTIFICATION") {
				p.textNodes = append(p.textNodes, node)
			} else {
				p.attachmentNodes = append(p.attachmentNodes, node)
			}
			return
		case "TEXT":
			if strings.Contains(typeStr, "/PLAIN") || strings.Contains(typeStr, "/HTML") {
				p.textNodes = append(p.textNodes, node)
			} else {
				p.attachmentNodes = append(p.attachmentNodes, node)
			}
			return
		default:
			p.attachmentNodes = append(p.attachmentNodes, node)
			return
		}
	}

	walkAllNode(p.topNode)
}

func (p *EmailParser) classifyAlternativeShowNodes() {
	if p.alternativeShowNodesDealed {
		return
	}
	p.alternativeShowNodesDealed = true
	p.classifyNodes()

	type alternativeSet struct {
		HTML  *MIMENode
		PLAIN *MIMENode
	}

	tmpShowVector := []*MIMENode{}
	tmpShowSet := make(map[string]*alternativeSet)

	for _, node := range p.textNodes {
		parts := strings.SplitN(node.ContentType, "/", 2)
		subType := ""
		if len(parts) == 2 {
			subType = parts[1]
		}
		if subType != "HTML" && subType != "PLAIN" {
			tmpShowVector = append(tmpShowVector, node)
			continue
		}
		alternativeKey := ""
		parent := node.Parent
		for parent != nil {
			if parent.ContentType == "MULTIPART/ALTERNATIVE" {
				alternativeKey = fmt.Sprintf("^_^%d", parent.HeaderStart)
				break
			}
			parent = parent.Parent
		}
		if alternativeKey == "" {
			tmpShowVector = append(tmpShowVector, node)
			continue
		}
		if _, ok := tmpShowSet[alternativeKey]; !ok {
			tmpShowSet[alternativeKey] = &alternativeSet{}
		}
		if subType == "HTML" {
			tmpShowSet[alternativeKey].HTML = node
		} else if subType == "PLAIN" {
			tmpShowSet[alternativeKey].PLAIN = node
		}
	}
	for alternativeKey, ns := range tmpShowSet {
		if !strings.HasPrefix(alternativeKey, "^_^") {
			continue
		}
		if ns.HTML != nil {
			tmpShowVector = append(tmpShowVector, ns.HTML)
		} else if ns.PLAIN != nil {
			tmpShowVector = append(tmpShowVector, ns.PLAIN)
		}
	}
	// Sort by HeaderStart
	sort.Slice(tmpShowVector, func(i, j int) bool {
		return tmpShowVector[i].HeaderStart < tmpShowVector[j].HeaderStart
	})
	p.alternativeShowNodes = tmpShowVector
}

func (p *EmailParser) GetTextNodes() []*MIMENode {
	if p.nodeClassified {
		return p.textNodes
	}
	p.classifyNodes()
	return p.textNodes
}

func (p *EmailParser) GetAttachmentNodes() []*MIMENode {
	if p.nodeClassified {
		return p.attachmentNodes
	}
	p.classifyNodes()
	return p.attachmentNodes
}

func (p *EmailParser) GetAlternativeShowNodes() []*MIMENode {
	if p.alternativeShowNodesDealed {
		return p.alternativeShowNodes
	}
	p.classifyAlternativeShowNodes()
	return p.alternativeShowNodes
}

func (p *EmailParser) classifyInlineAttachmentNodes() {
	if p.inlineAttachmentNodesDealed {
		return
	}
	p.inlineAttachmentNodesDealed = true
	p.classifyNodes()
	p.classifyAlternativeShowNodes()

	hasContentId := false
	for _, m := range p.attachmentNodes {
		if m.ContentID != "" {
			hasContentId = true
			break
		}
	}
	if !hasContentId {
		return
	}

	var conBuilder strings.Builder
	for _, n := range p.alternativeShowNodes {
		conBuilder.WriteString(string(GetDecodedContent(n)))
		conBuilder.WriteString("\n")
	}
	con := strings.ToLower(conBuilder.String())
	for _, m := range p.attachmentNodes {
		if m.ContentID == "" {
			continue
		}
		cid := strings.ToLower(m.ContentID)
		if strings.Contains(con, "cid:"+cid) {
			m.isInline = true
		}
	}
}
