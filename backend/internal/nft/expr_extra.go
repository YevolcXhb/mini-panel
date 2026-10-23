//go:build linux

package nft

import "encoding/binary"

// ─── 补充属性编号 ───

// bitwise 表达式
const (
	AttrBitwiseSreg = 1
	AttrBitwiseDreg = 2
	AttrBitwiseLen  = 3
	AttrBitwiseMask = 4
	AttrBitwiseXor  = 5
)

// ct 表达式
const (
	AttrCtDreg      = 1
	AttrCtKey       = 2
	AttrCtDirection = 3
)

// ct key
const (
	CtState   = 0
	CtStatus  = 1
	CtMark    = 2
	CtProto   = 9
	CtL3Proto = 11
)

// ct state 位
const (
	CtStateInvalid     = 1
	CtStateEstablished = 2
	CtStateRelated     = 4
	CtStateNew         = 8
	CtStateUntracked   = 64
)

// meta key
const (
	MetaKeyIIF     = 4  // 入接口 ifindex
	MetaKeyOIF     = 5  // 出接口 ifindex
	MetaKeyIIFName = 6  // 入接口名
	MetaKeyOIFName = 7  // 出接口名
	MetaKeyL4Proto = 16 // 四层协议号
	MetaKeyProtocol = 17
	MetaKeyMark    = 3
)

// MetaKey 读取任意 meta key 到寄存器
func MetaKey(key, reg uint32) Expr {
	var a attrBuf
	a.U32(AttrMetaKey, key)
	a.U32(AttrMetaDreg, reg)
	return Expr{Name: ExprMeta, Data: a.b}
}

// ─── 表达式构造 ───

// CTState 读取连接跟踪状态到寄存器
func CTState(reg uint32) Expr {
	var a attrBuf
	a.U32(AttrCtDreg, reg)
	a.U32(AttrCtKey, CtState)
	return Expr{Name: ExprConntrack, Data: a.b}
}

// BitwiseAnd 寄存器按位与，结果写入另一个寄存器。
//
// 掩码是嵌套 nft_data，载荷长度必须与 LEN 一致（4/8/16 字节）。
func BitwiseAnd(sreg, dreg uint32, mask []byte) Expr {
	var a attrBuf
	a.U32(AttrBitwiseSreg, sreg)
	a.U32(AttrBitwiseDreg, dreg)
	a.U32(AttrBitwiseLen, uint32(len(mask)))
	a.Nested(AttrBitwiseMask, func(m *attrBuf) {
		m.Raw(AttrDataValue, mask)
	})
	return Expr{Name: ExprBitwise, Data: a.b}
}

// BitwiseMaskXor 构造完整的 bitwise：dreg = (sreg & mask) ^ xor。
//
// 实测本内核要求 MASK 与 XOR 两个属性都出现，只给 MASK 会 EINVAL。
func BitwiseMaskXor(sreg, dreg uint32, mask, xor []byte) Expr {
	var a attrBuf
	a.U32(AttrBitwiseSreg, sreg)
	a.U32(AttrBitwiseDreg, dreg)
	a.U32(AttrBitwiseLen, uint32(len(mask)))
	a.Nested(AttrBitwiseMask, func(m *attrBuf) { m.Raw(AttrDataValue, mask) })
	a.Nested(AttrBitwiseXor, func(x *attrBuf) { x.Raw(AttrDataValue, xor) })
	return Expr{Name: ExprBitwise, Data: a.b}
}

// CmpU32BE 4 字节大端常量比较
func CmpU32BE(reg uint32, op uint32, v uint32) Expr {
	return Cmp(reg, op, []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

// IPv4ToBytes 把点分十进制转 4 字节；失败返回 nil
func IPv4ToBytes(s string) []byte {
	var a, b, c, d int
	n := 0
	cur := 0
	part := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= '0' && ch <= '9' {
			cur = cur*10 + int(ch-'0')
			n++
			continue
		}
		if ch != '.' {
			return nil
		}
		if n == 0 || cur > 255 {
			return nil
		}
		switch part {
		case 0:
			a = cur
		case 1:
			b = cur
		case 2:
			c = cur
		default:
			return nil
		}
		part++
		cur = 0
		n = 0
	}
	if n == 0 || cur > 255 || part != 3 {
		return nil
	}
	d = cur
	return []byte{byte(a), byte(b), byte(c), byte(d)}
}

// PrefixMask 生成 IPv4 前缀掩码（/n）。n 不在 0..32 返回 nil。
func PrefixMask(bits int) []byte {
	if bits < 0 || bits > 32 {
		return nil
	}
	m := uint32(0)
	if bits > 0 {
		m = ^uint32(0) << (32 - bits)
	}
	var out [4]byte
	binary.BigEndian.PutUint32(out[:], m)
	return out[:]
}

// ImmediateValue32 把 4 字节常量写入寄存器（IP 地址等）。
// NFTA_DATA_VALUE 里直接放大端字节。
func ImmediateValue32(reg uint32, value []byte) Expr {
	var data attrBuf
	data.Raw(AttrDataValue, value)

	var a attrBuf
	a.U32(AttrImmDreg, reg)
	a.Nested(AttrImmData, func(dst *attrBuf) {
		dst.b = append(dst.b, data.b...)
	})
	return Expr{Name: ExprImmediate, Data: a.b}
}

// ImmediateValue16 把 16 位常量写入寄存器（端口等，网络字节序）。
func ImmediateValue16(reg uint32, v uint16) Expr {
	return ImmediateValue32(reg, []byte{byte(v >> 8), byte(v)})
}
