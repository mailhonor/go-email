package emailparser

import (
	"bytes"
	"strings"
)

// ParseMimeAddress 解析MIME格式的地址行
func ParseMimeAddress(line []byte, defaultCharset string) []MimeAddress {
	var mas []MimeAddress
	tmpbf := make([]byte, len(line)+1) // 临时缓冲区
	bf := line

	for len(bf) > 0 {
		ma := parseMimeAddress_decodeOne(bf, tmpbf)
		if ma == nil {
			break
		}
		// 仅添加有效地址或名称
		if ma.address != "" || len(ma.nameBuffer) > 0 {
			mas = append(mas, MimeAddress{
				NameRaw: ma.nameBuffer,
				Email:   ma.address,
			})
		}
		bf = ma.leftbf
	}
	// 转换名称为字符串
	for i := range mas {
		mas[i].Name = strings.Trim(ParseMimeValueString(mas[i].NameRaw, defaultCharset), " \r\n\t\"'")
		mas[i].Email = strings.ToLower(strings.Trim(mas[i].Email, " \r\n\t\"'"))
	}
	return mas
}

// ParseMimeAddressFirstOne 解析MIME格式的地址行，仅返回第一个地址
func ParseMimeAddressFirstOne(line []byte, defaultCharset string) MimeAddress {
	mas := ParseMimeAddress(line, defaultCharset)
	if len(mas) > 0 {
		return mas[0]
	}
	return MimeAddress{}
}

// parseMimeAddress_decodeResult 内部解析结果结构体
type parseMimeAddress_decodeResult struct {
	leftbf     []byte
	nameBuffer []byte
	address    string
}

// parseMimeAddress_decodeOne 解析单个MIME地址
func parseMimeAddress_decodeOne(line []byte, tmpbf []byte) *parseMimeAddress_decodeResult {
	foundLT := false
	var name, address []byte
	i := 0
	tmpbfI := 0
	bf := line

	// 跳过开头的空格和制表符
	pos := parseMimeAddress_lineBufferSkipChar(bf, " \t")
	if pos < 0 {
		return nil
	}
	bf = bf[pos:]
	i = 0

	// 主解析循环
	for i < len(bf) && !foundLT {
		ch := bf[i]
		i++
		chStr := string(ch)

		if chStr == "\"" {
			// 处理引号内内容
			for i < len(bf) {
				ch = bf[i]
				i++
				chStr = string(ch)

				if chStr == "\\" {
					// 处理转义字符
					if i < len(bf) {
						tmpbf[tmpbfI] = bf[i]
						tmpbfI++
						i++
					}
					continue
				} else if chStr == "\"" {
					// 引号结束
					break
				} else {
					// 普通字符
					tmpbf[tmpbfI] = ch
					tmpbfI++
				}
			}

			// 到达缓冲区末尾时提取名称
			if i == len(bf) {
				name = make([]byte, tmpbfI)
				copy(name, tmpbf[:tmpbfI])
				tmpbfI = 0
				break
			}
		} else if chStr == "," || chStr == ";" {
			// 地址分隔符
			name = make([]byte, tmpbfI)
			copy(name, tmpbf[:tmpbfI])
			tmpbfI = 0
			break
		} else if chStr == "<" {
			// 开始解析邮箱地址
			foundLT = true
			// 提取名称
			name = make([]byte, tmpbfI)
			copy(name, tmpbf[:tmpbfI])
			tmpbfI = 0

			// 解析邮箱内容
			for i < len(bf) {
				ch = bf[i]
				i++
				chStr = string(ch)

				if chStr == "<" {
					// 处理嵌套的<
					tmpbf[tmpbfI] = byte(' ')
					tmpbfI++
					// 拼接名称
					name = append(append(name, byte('<')), tmpbf[:tmpbfI]...)
					tmpbfI = 0
					continue
				}

				// 检查邮箱结束符
				if chStr == "," || chStr == ";" || chStr == ">" {
					address = make([]byte, tmpbfI)
					copy(address, tmpbf[:tmpbfI])
					break
				}

				// 收集邮箱字符
				tmpbf[tmpbfI] = ch
				tmpbfI++

				// 到达末尾时提取邮箱
				if i == len(bf) {
					address = make([]byte, tmpbfI)
					copy(address, tmpbf[:tmpbfI])
					break
				}
			}
			continue
		} else {
			// 普通字符
			tmpbf[tmpbfI] = ch
			tmpbfI++

			// 到达末尾时提取名称
			if i == len(bf) {
				name = make([]byte, tmpbfI)
				copy(name, tmpbf[:tmpbfI])
				tmpbfI = 0
				break
			}
		}
	}

	// 剩余未解析的缓冲区
	leftbf := bf[i:]

	// 处理没有明确邮箱但名称包含@的情况
	if address == nil {
		if name != nil && bytes.Contains(name, []byte("@")) {
			address = name
			name = nil
		}
	}

	// 处理默认值
	if address == nil {
		address = []byte{}
	}
	if name == nil {
		name = []byte{}
	}

	// 修剪邮箱地址
	av := strings.TrimSpace(string(address))

	return &parseMimeAddress_decodeResult{
		leftbf:     leftbf,
		nameBuffer: name,
		address:    av,
	}
}

// parseMimeAddress_lineBufferSkipChar 跳过缓冲区开头的指定字符
func parseMimeAddress_lineBufferSkipChar(bf []byte, chars string) int {
	for i, b := range bf {
		if !strings.Contains(chars, string(b)) {
			return i
		}
	}
	return -1
}
