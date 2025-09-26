package emailparser

import (
	"bytes"
	"strconv"
	"strings"
	"unicode"

	mailhonorstringutils "github.com/mailhonor/go-utils/strings"
)

// ParseMimeValueParams 解析MIME冒号后的值和参数（无error返回，兼容所有异常）
// 输入：MIME行冒号后的原始字节数据（如 "text\"=;/plain"; charset="gbk;vvv"; ...）
// 特性：1. 支持二进制/非法UTF-8 2. 跳过/修复异常内容 3. 保留原始输入
func ParseMimeValueParams(data []byte) MimeValueParams {
	result := &MimeValueParams{
		Params: make(map[string][]byte),
	}

	// 预处理：去除开头空白，避免空输入问题
	content := mailhonorstringutils.TrimLeftBytes(data, []byte(" \t"))
	if len(content) == 0 {
		result.Value = []byte{}
		return *result
	}

	// 1. 解析主值（最大努力：即使有异常也保留可解析部分）
	valueEnd := ParseMimeValueParams_parseValue(content, result)

	// 2. 解析参数（最大努力：跳过异常参数，保留合法部分）
	if valueEnd < len(content) {
		// 预处理参数部分：去除开头的分号/空白
		paramsContent := bytes.TrimLeftFunc(content[valueEnd:], func(r rune) bool {
			return r == ';' || unicode.IsSpace(r)
		})
		ParseMimeValueParams_parseParams(paramsContent, result)
	}

	return *result
}

// parseValue 解析主值（无异常中断，异常时返回已解析部分的结束位置）
func ParseMimeValueParams_parseValue(content []byte, result *MimeValueParams) int {
	// 场景1：带引号的值（处理未闭合引号、转义异常）
	if len(content) > 0 && content[0] == '"' {
		closeQuoteIdx := -1
		// 遍历寻找合法闭合引号（跳过转义的引号）
		for i := 1; i < len(content); i++ {
			// 合法闭合：当前是引号且前一个不是反斜杠（或反斜杠本身被转义）
			if content[i] == '"' && (i == 0 || content[i-1] != '\\') {
				closeQuoteIdx = i
				break
			}
		}

		if closeQuoteIdx != -1 {
			// 正常闭合：处理转义（仅修复可识别的转义，异常转义保留原始）
			valueRaw := content[1:closeQuoteIdx]
			result.Value = ParseMimeValueParams_unescapeQuotedBytes(valueRaw)
			return closeQuoteIdx + 1 // 跳过闭合引号
		} else {
			// 异常：未闭合引号，取引号后所有内容（去除末尾可能的残缺引号）
			valueRaw := bytes.TrimSuffix(content[1:], []byte{'"'})
			result.Value = ParseMimeValueParams_unescapeQuotedBytes(valueRaw)
			return len(content) // 已处理完所有内容
		}
	}

	// 场景2：不带引号的值（到分号/空白结束，无异常）
	for i := 0; i < len(content); i++ {
		r := rune(content[i])
		if content[i] == ';' || unicode.IsSpace(r) {
			result.Value = content[:i]
			return i
		}
	}

	// 场景3：整个内容都是值（无终止符，直接取全部）
	result.Value = content
	return len(content)
}

// parseParams 解析参数（无异常中断，跳过非法参数）
func ParseMimeValueParams_parseParams(content []byte, result *MimeValueParams) {
	current := content
	for len(current) > 0 {
		// 步骤1：解析参数名（跳过空名、非法名）
		nameEnd := 0
		for nameEnd < len(current) {
			b := current[nameEnd]
			r := rune(b)
			// 参数名终止符：等号、分号、空白（避免特殊字符）
			if b == '=' || b == ';' || unicode.IsSpace(r) {
				break
			}
			nameEnd++
		}
		// 跳过空参数名（如连续分号导致）
		name := bytes.TrimSpace(current[:nameEnd])
		if len(name) == 0 {
			current = ParseMimeValueParams_skipToNextParam(current[nameEnd:])
			continue
		}

		// 步骤2：定位等号（无等号则跳过该参数）
		current = current[nameEnd:]
		current = bytes.TrimLeftFunc(current, unicode.IsSpace)
		if len(current) == 0 || current[0] != '=' {
			current = ParseMimeValueParams_skipToNextParam(current)
			continue
		}
		// 跳过等号
		current = current[1:]
		current = bytes.TrimLeftFunc(current, unicode.IsSpace)

		// 步骤3：解析参数值（兼容带引号/不带引号，异常时取默认值）
		var value []byte
		var valueLen int
		if len(current) > 0 && current[0] == '"' {
			// 带引号值（处理未闭合、转义异常）
			value, valueLen = ParseMimeValueParams_parseQuotedParamValue(current)
		} else {
			// 不带引号值（到分号/空白结束）
			value, valueLen = ParseMimeValueParams_parseUnquotedParamValue(current)
		}

		// 步骤4：存储参数（参数名小写，避免重复）
		lowerName := bytes.ToUpper(name)
		// 避免重复参数：保留第一个合法值
		if _, exists := result.Params[string(lowerName)]; !exists {
			result.Params[string(lowerName)] = value
		}

		// 步骤5：移动到下一个参数
		current = current[valueLen:]
		current = ParseMimeValueParams_skipToNextParam(current)
	}
}

// parseQuotedParamValue 解析带引号参数值（无error，返回值+已处理长度）
func ParseMimeValueParams_parseQuotedParamValue(content []byte) ([]byte, int) {
	if len(content) == 0 || content[0] != '"' {
		// 异常：不是引号开头，按不带引号值处理
		return ParseMimeValueParams_parseUnquotedParamValue(content)
	}

	// 寻找闭合引号（异常时取全部内容）
	closeQuoteIdx := -1
	for i := 1; i < len(content); i++ {
		if content[i] == '"' && (i == 0 || content[i-1] != '\\') {
			closeQuoteIdx = i
			break
		}
	}

	if closeQuoteIdx != -1 {
		// 正常闭合：处理转义
		value := ParseMimeValueParams_unescapeQuotedBytes(content[1:closeQuoteIdx])
		return value, closeQuoteIdx + 1
	} else {
		// 异常：未闭合引号，取引号后所有内容（去除末尾残缺引号）
		value := ParseMimeValueParams_unescapeQuotedBytes(bytes.TrimSuffix(content[1:], []byte{'"'}))
		return value, len(content)
	}
}

// parseUnquotedParamValue 解析不带引号参数值（无异常，返回值+已处理长度）
func ParseMimeValueParams_parseUnquotedParamValue(content []byte) ([]byte, int) {
	for i := 0; i < len(content); i++ {
		r := rune(content[i])
		if content[i] == ';' || unicode.IsSpace(r) {
			return content[:i], i
		}
	}
	// 无终止符，取全部内容
	return content, len(content)
}

// unescapeQuotedBytes 处理转义字符（仅修复合法转义，异常转义保留原始）
func ParseMimeValueParams_unescapeQuotedBytes(b []byte) []byte {
	var result []byte
	i := 0
	for i < len(b) {
		// 仅处理可识别的转义：\"（转义引号）、\\（转义反斜杠）
		if b[i] == '\\' && i+1 < len(b) {
			switch b[i+1] {
			case '"', '\\':
				result = append(result, b[i+1])
				i += 2 // 跳过转义符和目标符
				continue
			}
		}
		// 非转义字符/异常转义，直接保留
		result = append(result, b[i])
		i++
	}
	return result
}

// skipToNextParam 跳过当前异常内容，定位到下一个参数的起始位置（分号后）
func ParseMimeValueParams_skipToNextParam(content []byte) []byte {
	// 找到下一个分号
	semicolonIdx := bytes.IndexByte(content, ';')
	if semicolonIdx == -1 {
		return []byte{} // 无下一个参数，返回空
	}
	// 跳过分号并去除后续空白
	return bytes.TrimLeftFunc(content[semicolonIdx+1:], unicode.IsSpace)
}

// TrimmedValue 返回主值经过trim处理后的版本（去除首尾空白）
func (m *MimeValueParams) TrimmedValue() []byte {
	if m == nil {
		return []byte{}
	}
	v := m.Value
	v = mailhonorstringutils.TrimBytes(v, []byte("\r\n\t \"'"))
	return v
}

// TrimmedParam 返回指定参数经过trim处理后的值（去除首尾空白）
// 若参数不存在，返回空字节切片
func (m *MimeValueParams) TrimmedParam(key string) []byte {
	if m == nil || m.Params == nil {
		return []byte{}
	}
	// 参数名转为小写，符合MIME参数名大小写不敏感的规范
	val, ok := m.Params[strings.ToUpper(key)]
	if !ok {
		return []byte{}
	}
	return mailhonorstringutils.TrimBytes(val, []byte("\r\n\t \"'"))
}

func (m *MimeValueParams) ParamExist(key string) bool {
	if m == nil || m.Params == nil {
		return false
	}
	_, ok := m.Params[strings.ToUpper(key)]
	return ok
}

func (m *MimeValueParams) ParseParamStringValue(key string, defaultCharset string) string {
	key = strings.ToUpper(key)
	if m == nil || m.Params == nil {
		return ""
	}
	//
	val, ok := m.Params[strings.ToUpper(key)]
	if ok {
		return ParseMimeValueString(val, defaultCharset)
	}
	//
	val, ok = m.Params[key+"*"]
	if ok {
		// 查看 val 中有多少 "'"
		count := 0
		for i := 0; i < len(val); i++ {
			if val[i] == '\'' {
				count++
			}
		}
		return ParseMimeValueString2231(val, count == 2, defaultCharset)
	}
	//
	val, ok = m.Params[key+"*0*"]
	if ok {
		vals := [][]byte{val}
		for i := 1; ; i++ {
			nextKey := key + "*" + strconv.Itoa(i) + "*"
			val, ok = m.Params[nextKey]
			if !ok {
				break
			}
			vals = append(vals, val)
		}
		return ParseMimeValueString2231(mailhonorstringutils.ConcatByteSlices(vals), true, defaultCharset)
	}

	//
	val, ok = m.Params[key+"*0"]
	if ok {
		vals := [][]byte{val}
		for i := 1; ; i++ {
			nextKey := key + "*" + strconv.Itoa(i)
			val, ok = m.Params[nextKey]
			if !ok {
				break
			}
			vals = append(vals, val)
		}
		return ParseMimeValueString2231(mailhonorstringutils.ConcatByteSlices(vals), false, defaultCharset)
	}
	//
	return ""
}
