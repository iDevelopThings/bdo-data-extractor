package schema

import "github.com/idevelopthings/bdo-data-extractor/internal/bss"

// Buff is the buff.dbss record schema; it consumes every record exactly.
// EffectData is the buff module's argument container — flags[7] + ten
// {i32 value, i32 aux} slots + flags[5] — read per ModuleType by
// tables.ResolveBuffStat (see internal/tables/buffmodules.go and FORMATS.md §6).
var Buff = bss.New("buff").
	Add(bss.UInt16, "Index").
	Add(bss.Text, "Name").
	Add(bss.Int16, "Category").
	Add(bss.Byte, "CategoryLevel").
	Add(bss.Byte, "Level").
	Add(bss.Int16, "Group").
	Add(bss.Int16, "ConditionType").
	Add(bss.Byte, "ModuleType").
	Add(bss.Byte, "BuffType").
	Add(bss.Byte, "IsAbsolute").
	Add(bss.Byte, "IsOverlapped").
	AddBytesNamed("EffectData", 92).
	Add(bss.Int32, "DurationMs").
	AddBytes(25).
	Add(bss.Text, "ApplyToGroup").
	Add(bss.UtfText, "Icon").
	Anon(bss.Byte).
	Anon(bss.Int32).
	Add(bss.Text, "Desc").
	AddBytes(27)

func init() { register(Buff) }
