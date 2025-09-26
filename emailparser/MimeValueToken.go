package emailparser

import (
	"bytes"
	"encoding/base64"
	"regexp"
	"strings"

	mailhonorcharsetutils "github.com/mailhonor/go-utils/charset"
	mailhonorquotedprintableutils "github.com/mailhonor/go-utils/quotedprintable"
	mailhonorstringutils "github.com/mailhonor/go-utils/strings"
)

// tokenNodeWithEncoding 带有编码信息的中间令牌节点（内部临时使用）
type tokenNodeWithEncoding struct {
	Charset  string
	Data     []byte
	Encoding string
}

// ParseMimeValueTokenNodes 解析 MIME 编码的头部值，返回令牌节点列表
// 功能：识别 "=?charset?encoding?data?=" 格式片段，合并相同编码/字符集的连续节点
// 注：Base64/QP 解码仅保留函数框架，暂不执行实际解码（返回原始数据）
func ParseMimeValueTokenNodes(line []byte) []MimeValueTokenNode {
	var rs []tokenNodeWithEncoding

	// 内部辅助函数：添加节点并合并相同编码/字符集的连续节点
	push := func(item tokenNodeWithEncoding) {
		if item.Charset == "" || len(rs) == 0 {
			rs = append(rs, item)
			return
		}

		last := &rs[len(rs)-1]
		if last.Charset == item.Charset && last.Encoding == item.Encoding {
			// 合并数据（避免重复创建节点）
			last.Data = append(last.Data, item.Data...)
		} else {
			rs = append(rs, item)
		}
	}

	var bf, bfBegin []byte = line, line
	var pos int
	magicOffset := 0 // 处理异常场景的偏移量（跳过无效的 "=?" 标记）

	// 循环解析 MIME 编码片段和普通文本
	for len(bfBegin) > 0 {
		bf = bfBegin
		// 查找 MIME 编码起始标记 "=?"（支持偏移量修正）
		pos = bytes.Index(bf[magicOffset:], []byte("=?"))
		if pos != -1 {
			pos += magicOffset // 修正为全局位置
		}
		magicOffset = 0 // 重置偏移量

		// 场景1：无编码标记，剩余内容全部作为普通文本
		if pos < 0 {
			push(tokenNodeWithEncoding{
				Charset:  "",
				Data:     bytes.Clone(bf),
				Encoding: "",
			})
			break
		}

		// 场景2：编码标记前有普通文本，先处理普通文本
		if pos > 0 {
			push(tokenNodeWithEncoding{
				Charset:  "",
				Data:     bytes.Clone(bf[:pos]),
				Encoding: "",
			})
			bf = bf[pos:] // 聚焦到编码标记部分
		}

		bfBegin = bf
		bf = bf[2:] // 跳过 "=?" 标记

		// 查找字符集结束的 "?"（字符集至少需2个字符才合法）
		pos = bytes.IndexByte(bf, '?')
		if pos < 2 {
			magicOffset = 2 // 标记为无效标记，下一轮从偏移2开始
			continue
		}

		// 提取字符集（转为大写，统一格式）
		charset := strings.ToUpper(string(bf[:pos]))
		bf = bf[pos+1:] // 跳过字符集后的 "?"

		// 检查剩余长度是否满足编码标识格式（至少需4字节：编码+?+数据）
		if len(bf) < 4 {
			magicOffset = 2
			continue
		}

		// 提取编码方式（仅支持 B/base64 或 Q/quoted-printable）
		encoding := strings.ToUpper(string(bf[0]))
		if encoding != "B" && encoding != "Q" {
			magicOffset = 2
			continue
		}

		// 验证编码标识后的 "?" 是否存在
		if bf[1] != '?' {
			magicOffset = 2
			continue
		}

		bf = bf[2:] // 跳过编码标识和 "?"

		// 场景3：查找编码片段结束标记 "?="
		pos = bytes.Index(bf, []byte("?="))
		if pos > -1 {
			bfBegin = bf[pos+2:] // 跳过 "?="，更新下一轮起始位置
			push(tokenNodeWithEncoding{
				Charset:  charset,
				Data:     bytes.Clone(bf[:pos]),
				Encoding: encoding,
			})
			continue
		}

		// 场景4：未找到 "?="，以空格/制表符为分隔符（MIME 允许的空格分隔）
		pos = lineBufferFindChar(bf, " \t")
		if pos > -1 {
			bfBegin = bf[pos:] // 跳过分隔符，更新下一轮起始位置
			push(tokenNodeWithEncoding{
				Charset:  charset,
				Data:     bytes.Clone(bf[:pos]),
				Encoding: encoding,
			})
			continue
		}

		// 场景5：无结束标记和分隔符，剩余内容作为编码数据
		push(tokenNodeWithEncoding{
			Charset:  charset,
			Data:     bytes.Clone(bf),
			Encoding: encoding,
		})
		break
	}

	// 转换为最终输出的 MimeValueTokenNode（解码函数仅留框架）
	var newRs []MimeValueTokenNode
	for _, item := range rs {
		var data []byte
		switch item.Encoding {
		case "B":
			data = decodeHeaderBase64(item.Data) // 空实现：返回原始数据
		case "Q":
			data = mailhonorquotedprintableutils.DecodeMimeHeader(item.Data) // 空实现：返回原始数据
		default:
			data = item.Data // 无编码：直接保留原始数据
		}
		newRs = append(newRs, MimeValueTokenNode{
			Charset: item.Charset,
			Data:    data,
		})
	}

	return newRs
}

// lineBufferFindChar 查找字节数组中第一个匹配指定字符集的位置
// 参数 chars：字符集合（如 " \t" 表示匹配空格或制表符）
func lineBufferFindChar(bf []byte, chars string) int {
	for i, b := range bf {
		if strings.Contains(chars, string(b)) {
			return i
		}
	}
	return -1
}

// DecodeHeaderBase64 解码 Base64 编码的头部数据
// 处理包含多个等号分隔的 Base64 片段，合并解码结果
func decodeHeaderBase64(data []byte) []byte {
	var result [][]byte
	b := string(data) // 将输入字节转换为字符串处理

	for len(b) > 0 {
		// 查找等号位置
		pos := strings.Index(b, "=")
		if pos < 0 {
			// 没有等号，整个剩余字符串作为 Base64 数据解码
			decoded, _ := base64.StdEncoding.DecodeString(b)
			result = append(result, decoded)
			break
		}

		// 移动到等号后一位
		pos++

		// 跳过连续的等号
		for pos < len(b) && b[pos] == '=' {
			pos++
		}

		// 解码从开始到当前位置的片段
		segment := b[:pos]
		decoded, _ := base64.StdEncoding.DecodeString(segment)
		result = append(result, decoded)

		// 处理剩余部分
		b = b[pos:]
	}

	// 合并所有解码后的片段
	return mailhonorstringutils.ConcatByteSlices(result)
}

var hexRegex = regexp.MustCompile(`^[\da-fA-F]{2}$`)

func ParseMimeValueTokenNodes2231(line []byte, withCharset bool) []MimeValueTokenNode {
	bf := line

	if !withCharset {
		return ParseMimeValueTokenNodes(bf)
	}

	// 查找第一个单引号位置
	pos := bytes.IndexByte(bf, '\'')
	if pos < 0 {
		// 没有找到单引号，返回原始数据
		return []MimeValueTokenNode{{
			Charset: "",
			Data:    bf,
		}}
	}

	// 提取字符集并转为大写
	charset := strings.TrimSpace(string(bf[:pos]))
	charset = strings.ToUpper(charset)

	// 跳过第一个单引号
	bf = bf[pos+1:]

	// 查找第二个单引号位置
	pos = bytes.IndexByte(bf, '\'')
	if pos > -1 {
		// 跳过第二个单引号
		bf = bf[pos+1:]
	}

	// 处理百分号编码
	str := string(bf)
	tmpbf := make([]byte, len(str))
	tmpbfI := 0

	for i := 0; i < len(str); i++ {
		chr := str[i]
		// 检查是否为百分号编码
		if chr == '%' && i+2 < len(str) {
			hex := str[i+1 : i+3]
			if hexRegex.MatchString(hex) {
				// 解析十六进制值
				val := 0
				for _, c := range hex {
					val <<= 4
					switch {
					case c >= '0' && c <= '9':
						val += int(c - '0')
					case c >= 'A' && c <= 'F':
						val += int(c - 'A' + 10)
					case c >= 'a' && c <= 'f':
						val += int(c - 'a' + 10)
					}
				}
				tmpbf[tmpbfI] = byte(val)
				tmpbfI++
				i += 2 // 跳过已处理的两个字符
				continue
			}
		}
		// 普通字符直接添加
		tmpbf[tmpbfI] = byte(chr)
		tmpbfI++
	}

	// 返回处理结果
	return []MimeValueTokenNode{{
		Charset: charset,
		Data:    tmpbf[:tmpbfI],
	}}
}

func ParseMimeValueString(line []byte, defaultCharset string) string {
	r := ""
	nodes := ParseMimeValueTokenNodes(line)
	for _, node := range nodes {
		r += mailhonorcharsetutils.ConvertToUTF8(node.Data, node.Charset, defaultCharset)
	}
	return r
}

func ParseMimeValueString2231(line []byte, withCharset bool, defaultCharset string) string {
	r := ""
	nodes := ParseMimeValueTokenNodes2231(line, withCharset)
	for _, node := range nodes {
		r += mailhonorcharsetutils.ConvertToUTF8(node.Data, node.Charset, defaultCharset)
	}
	return r
}
