package model

import (
	"log"
	"regexp"
	"strings"

	"github.com/idevelopthings/bdo-data-extractor/src/utils"
)

// EffectDsl is one parsed DSL formula from an enchant record, e.g. HP_UP(110).
// Args are float64 because some formulas carry fractional values (e.g.
// ALCHEMY_REDUCE_TIME_DOWN(0.7)); whole numbers still marshal as `4`, not `4.0`.
type EffectDsl struct {
	Func string    `json:"func"`
	Args []float64 `json:"args,omitempty"`
}

type EffectGroup struct {
	Title string    `json:"title"`
	Stats []StatMod `json:"stats"`
}

// StatMod is one parsed effect line, e.g. "Fishing EXP +10%" ->
// {Stat:"Fishing EXP", Op:"+", Value:10, Unit:"%"}.
type StatMod struct {
	*EffectDsl

	Stat   string  `json:"stat,omitempty"`
	Op     string  `json:"op,omitempty"`
	Value  float64 `json:"value,omitempty"`
	Unit   string  `json:"unit,omitempty"`
	Buff   uint32  `json:"buff,omitempty"`   // source buff Index (traceability)
	Negate bool    `json:"negate,omitempty"` // true if less is better, ie weight
	Note   string  `json:"note,omitempty"`   // optional consumer-facing note (e.g. "hidden stat")
}

// EffectSectionMarker are DSL funcs that start a new display section rather
// than carry a value.
var EffectSectionMarker = map[string]string{
	"ITEM_EFFECT":            "Effects",
	"POTENTIAL_EFFECT":       "Enhancement Effect",
	"POTENTIAL_EFFECT_START": "Enhancement Effect",
	"ADD_EFFECT":             "Additional Effect",
	"ADD_EFFECT_START":       "Additional Effect",
	"SPECIAL_EFFECT":         "Special Effect",
	"SPECIAL_EFFECT2":        "Special Effect",
	"INSTALL_EFFECT":         "Install Effect",
	"WEAR_EFFECT":            "Set Effect",
	"SET_EFFECT":             "Set Effect",
	// Alchemy stones and gathering tools mint their own set-bonus marker
	// instead of reusing SET_EFFECT.
	"ALCHEMY_4_SET": "Set Effect",
	"COLLECT_4_SET": "Set Effect",
	// Costume sets with their own named marker instead of a numbered NO_N_SET_EFFECT.
	"SET_DECORATE_COOK":    "Costume Set Effect",
	"SET_DECORATE_TERMIAN": "Summer Outfit Set Effect",
}

// sectionMarkerFor resolves the display section a marker func starts, if any.
// SET_EFFECT/WEAR_EFFECT/CASH_UP match by substring (not just the exact keys
// above) since boss and costume gear mint per-item variants —
// ANCIENT_NO_2_SET_EFFECT, BLACKSTAR_NO_3_SET_EFFECT_1, NO_6_WEAR_EFFECT,
// NO_8_CASH_UP, ... — that all belong under the same "Set Effect" section.
func sectionMarkerFor(fn string) (string, bool) {
	if title, ok := EffectSectionMarker[fn]; ok {
		return title, true
	}
	if strings.Contains(fn, "SET_EFFECT") || strings.Contains(fn, "WEAR_EFFECT") || strings.Contains(fn, "CASH_UP") {
		return "Set Effect", true
	}
	return "", false
}

// EffectFuncInfo is a DSL func's display info; only what formatting needs
// (the gear builder's stat aggregation keeps its own stat/apStat mapping —
// see frontend/src/lib/effect-dsl.ts).
type EffectFuncInfo struct {
	label  string
	unit   string
	negate bool // "..._DOWN" funcs carry positive args but reduce the stat (time costs)
}

var EffectFuncs = map[string]EffectFuncInfo{
	"MONSTER_DAM_ADD":                 {label: "Extra AP Against Monsters"},
	"PLAYER_DAM_ADD":                  {label: "Extra AP Against Adventurers"},
	"P_H_DAM_ADD":                     {label: "Extra AP Against All"},
	"P_M_DAM_ADD":                     {label: "Extra AP Against All"},
	"ALL_TRIBE_DAM_ADD_NOHUMAN":       {label: "Extra AP Against All (except Humans)"},
	"ALL_TRIBE_DAM_ADD_NOHUMAN_NOAIN": {label: "Extra AP Against All (except Humans)"},
	"AIN_DAM_ADD":                     {label: "Extra AP Against Ahibs"},
	"KAMASILVIA_DAM_ADD":              {label: "Extra AP Against Kamasylvian"},
	"ALL_AP_UP":                       {label: "AP"},

	"ATT_UP":                  {label: "Attack Speed"},
	"CAS_UP":                  {label: "Casting Speed"},
	"CRI_POINT":               {label: "Critical Hit"},
	"CRI_ATT_DAM_ADD":         {label: "Critical Hit Extra Damage", unit: "%"},
	"ALL_HIT_UP":              {label: "Accuracy"},
	"ACC_ADD":                 {label: "Accuracy"},
	"ALL_SPECIAL_ATT_DAM_ADD": {label: "All Special Attack Damage", unit: "%"},

	"ALL_EVA_UP":         {label: "Evasion"},
	"ALL_DP_UP":          {label: "DP"},
	"ALL_DAM_REDUCE_ADD": {label: "Damage Reduction"},
	"MON_DAM_REDUCE_ADD": {label: "Monster Damage Reduction"},

	"HP_UP":         {label: "Max HP"},
	"HP_ADD":        {label: "Max HP"},
	"MP_WP_SP_UP":   {label: "Max MP/WP/SP"},
	"ENDURANCE_UP":  {label: "Max Stamina"},
	"ENDURANCE_ADD": {label: "Max Stamina"},

	"ALL_REG_ADD": {label: "All Resistance", unit: "%"},
	// Boss-gear (Kutum/Nouver) resistance bonus - the extractor now resolves
	// these to a real Args value (previously always empty; the +10% in-game
	// wasn't data-derived), so this is a normal valued lookup like ALL_REG_ADD.
	"KU_ALL_REG_ADD":                  {label: "All Resistance", unit: "%"},
	"NU_ALL_REG_ADD":                  {label: "All Resistance", unit: "%"},
	"STUN_STIFFNESS_FREEZING_REG_ADD": {label: "Stun/Stiffness/Freezing Resistance", unit: "%"},
	"KNOCKDOWN_BOUND_REG_ADD":         {label: "Knockdown/Bound Resistance", unit: "%"},
	"KNOCKBACK_AIRBORNE_REG_ADD":      {label: "Knockback/Floating Resistance", unit: "%"},

	"MOVE_UP":                    {label: "Movement Speed"},
	"MOVE_ADD":                   {label: "Movement Speed"},
	"LUCK_POINT_UP":              {label: "Luck"},
	"COMBAT_EXP_ACQUISITION_ADD": {label: "Combat EXP", unit: "%"},
	"SKILL_EXP_ACQUISITION_ADD":  {label: "Skill EXP", unit: "%"},
	"DEATH_DISAD_DOWN":           {label: "Death Penalty Reduction", unit: "%"},
	"AFFINITY_ACQUISITION_ADD":   {label: "Amity Gain", unit: "%"},
	"COLLECT_TIME_DECRE":         {label: "Gathering Time Reduction"},
	"JUMP_HEIGHT_ADD":            {label: "Jump Height"},

	// Haetae's blessing is the weight-limit effect (Basilisk's Belt +80 LT).
	"HAETAE_BLESSING":        {label: "Weight Limit", unit: "LT"},
	"HAETAE_BLESSING_SIMPLE": {label: "Weight Limit", unit: "LT"},

	// Life mastery + life stats on tools/clothes (Loggia/Manos etc.).
	"LIFESTAT_ALL":                   {label: "All Life Skill Mastery"},
	"LIFESTAT_ALL_ADD":               {label: "All Life Skill Mastery"},
	"LIFESTAT_ALCHEMY_ALL_ADD":       {label: "Alchemy Mastery"},
	"LIFESTAT_ALCHEMYPOINT_ALL_ADD":  {label: "Alchemy Mastery"},
	"LIFESTAT_COOK_ALL_ADD":          {label: "Cooking Mastery"},
	"LIFESTAT_COOK_ADD":              {label: "Cooking Mastery"},
	"LIFESTAT_FISHING_ALL_ADD":       {label: "Fishing Mastery"},
	"LIFESTAT_FISHINGPOINT_ALL_ADD":  {label: "Fishing Mastery"},
	"FISHING_POINT":                  {label: "Fishing Mastery"},
	"LIFESTAT_HUNTING_ALL_ADD":       {label: "Hunting Mastery"},
	"LIFESTAT_HUNTINGPOINT_ALL_ADD":  {label: "Hunting Mastery"},
	"LIFESTAT_VOYAGE_ALL_ADD":        {label: "Sailing Mastery"},
	"LIFESTAT_VOYAGEPOINT_ALL_ADD":   {label: "Sailing Mastery"},
	"LIFESTAT_TRAINING_ALL_ADD":      {label: "Training Mastery"},
	"LIFESTAT_TRAININGPOINT_ALL_ADD": {label: "Training Mastery"},
	"LIFESTAT_CRAFT":                 {label: "Processing Mastery"},
	"LIFESTAT_CRAFT_ADD":             {label: "Processing Mastery"},
	"LIFESTAT_CRAFT_ALL_ADD":         {label: "Processing Mastery"},
	"LIFESTAT_COLLECT_ALL":           {label: "Gathering Mastery"},
	"LIFESTAT_COLLECT_ALL_ADD":       {label: "Gathering Mastery"},
	// Not renamed despite the item-description audit below suggesting "Alchemy/Cooking
	// Mastery" for these two - their examples (Manos/Gorgath/Loggia Alchemist's Clothes)
	// also carry LIFESTAT_ALCHEMY_ALL_ADD, and the item's combined description text
	// doesn't distinguish which func contributes which line. The func name itself
	// (paralleling AUTO_FISHING_REDUCE_TIME_DOWN, CULTURE_REDUCE_TIME_DOWN) and BDO's
	// well-documented cooking/alchemy-time-reduction gear say this is a time reduction.
	"COOK_REDUCE_TIME_DOWN":    {label: "Cooking Time", unit: "sec", negate: true},
	"ALCHEMY_REDUCE_TIME_DOWN": {label: "Alchemy Time", unit: "sec", negate: true},
	"LIFE_EXP_POINT_ADD":       {label: "All Life Skill Mastery"},

	// LIFE_EXP_N/PO_LIFE_EXP_N/LIFE_STAT_N are NOT a generic "Life EXP" stat -
	// each numbered index is a distinct life skill, confirmed via items literally
	// named "<name>'s Artifact - <Skill> EXP/Mastery" (e.g. "Sethra's Artifact -
	// Gathering EXP"). LIFE_EXP_* grants EXP; LIFE_STAT_*/PO_LIFE_EXP_* grant Mastery.
	"LIFE_EXP_0":      {label: "All Life Skill EXP", unit: "%"},
	"LIFE_EXP_1":      {label: "Gathering EXP", unit: "%"},
	"LIFE_EXP_2":      {label: "Fishing EXP", unit: "%"},
	"LIFE_EXP_3":      {label: "Hunting EXP", unit: "%"},
	"LIFE_EXP_4":      {label: "Cooking EXP", unit: "%"},
	"LIFE_EXP_4_5":    {label: "Cooking EXP", unit: "%"},
	"LIFE_EXP_5":      {label: "Alchemy EXP", unit: "%"},
	"LIFE_EXP_6":      {label: "Processing EXP", unit: "%"},
	"LIFE_EXP_7":      {label: "Training EXP", unit: "%"},
	"LIFE_EXP_8":      {label: "Trading EXP", unit: "%"},
	"LIFE_EXP_9":      {label: "Farming EXP", unit: "%"},
	"LIFE_EXP_10":     {label: "Sailing EXP", unit: "%"},
	"LIFE_EXP_11":     {label: "Barter EXP", unit: "%"},
	"LIFE_STAT_0":     {label: "All Life Skill Mastery"},
	"LIFE_STAT_1":     {label: "Gathering Mastery"},
	"LIFE_STAT_2":     {label: "Fishing Mastery"},
	"LIFE_STAT_3":     {label: "Hunting Mastery"},
	"LIFE_STAT_4":     {label: "Cooking Mastery"},
	"LIFE_STAT_5":     {label: "Alchemy Mastery"},
	"LIFE_STAT_6":     {label: "Processing Mastery"},
	"LIFE_STAT_7":     {label: "Training Mastery"},
	"LIFE_STAT_8":     {label: "Sailing Mastery"},
	"PO_LIFE_EXP_1":   {label: "Gathering Mastery"},
	"PO_LIFE_EXP_2":   {label: "Fishing Mastery"},
	"PO_LIFE_EXP_3":   {label: "Hunting Mastery"},
	"PO_LIFE_EXP_4":   {label: "Cooking Mastery"},
	"PO_LIFE_EXP_5":   {label: "Alchemy Mastery"},
	"PO_LIFE_EXP_6":   {label: "Processing Mastery"},
	"PO_LIFE_EXP_6_1": {label: "Processing Mastery"},
	"PO_LIFE_EXP_7":   {label: "Training Mastery"},
	"PO_LIFE_EXP_7_1": {label: "Training Mastery"},
	"PO_LIFE_EXP_10":  {label: "Sailing Mastery"},

	// Combined Attack + Casting Speed (some accessories grant both at once).
	"ATT_CAS_ADD":   {label: "Attack & Casting Speed"},
	"ATT_CAS_UP":    {label: "Attack & Casting Speed"},
	"ATT_CAS_MINUS": {label: "Attack & Casting Speed", negate: true},

	"ALL_AP_DOWN":    {label: "AP", negate: true},
	"MOVE_SUB":       {label: "Movement Speed", negate: true},
	"ENDURANCE_DOWN": {label: "Max Stamina", negate: true},

	"CRI_ADD":            {label: "Critical Hit"},
	"HUMAN_DAM_ADD":      {label: "Extra AP Against Adventurers"},
	"MONSTER_DAM_REDUCE": {label: "Monster Damage Reduction"},

	"BACK_ATT_DAM_ADD":   {label: "Back Attack Damage"},
	"DOWN_ATT_DAM_ADD":   {label: "Down Attack Damage"},
	"BLEEDING__DAM_ADD":  {label: "Bleeding Damage"},
	"DEATH_DISAD_DOWN_1": {label: "Death Penalty Reduction", unit: "%", negate: true},

	// Range-specific combat stats (melee/ranged/magic split) - confirmed via
	// item names ("Marsh's/Lesha's Artifact - Melee/Ranged/Magic <stat>"): SHORT
	// = melee, LONG = ranged, not literally "short/long-range" as the func names
	// alone would suggest.
	"SHORT_AP_UP":          {label: "Melee AP"},
	"SHORT_EVA_UP":         {label: "Melee Evasion"},
	"SHORT_HIT_UP":         {label: "Melee Accuracy"},
	"SHORT_DAM_REDUCE_ADD": {label: "Melee Damage Reduction"},
	"LONG_AP_UP":           {label: "Ranged AP"},
	"LONG_EVA_UP":          {label: "Ranged Evasion"},
	"LONG_HIT_UP":          {label: "Ranged Accuracy"},
	"LONG_DAM_REDUCE_ADD":  {label: "Ranged Damage Reduction"},
	"MAGIC_AP_UP":          {label: "Magic AP"},
	"MAGIC_EVA_UP":         {label: "Magic Evasion"},
	"MAGIC_HIT_UP":         {label: "Magic Accuracy"},
	"MAGIC_DAM_REDUCE_ADD": {label: "Magic Damage Reduction"},

	// HP/resource recovery.
	"HP_RECOV_NATURAL":       {label: "Natural HP Recovery"},
	"ATT_HP_RECOV_POINT":     {label: "HP Recovery on Hit"},
	"MP_WP_SP_RECOV_NATURAL": {label: "Natural MP/WP/SP Recovery"},

	// Durability.
	"DUR_INCRE":                        {label: "Max Durability"},
	"MAX_DUR_ADD":                      {label: "Max Durability"},
	"DUR_WEAPONS_CON_DOWN":             {label: "Weapon Durability Consumption", unit: "%", negate: true},
	"DUR_AWAKEN_WEAPONS_CON_DOWN":      {label: "Awakening Weapon Durability Consumption", unit: "%", negate: true},
	"DUR_AWAKEN_WEAPONS_CON_DOWN_SHAI": {label: "Awakening Weapon Durability Consumption", unit: "%", negate: true},
	"ITEM_DESTROY_SUCC_DOWN":           {label: "Item Destruction Chance", unit: "%", negate: true},

	// Life-skill EXP bonuses, parallel to COMBAT_EXP_ACQUISITION_ADD/SKILL_EXP_ACQUISITION_ADD.
	"FISHING_EXP_POINT_ADD":     {label: "Fishing EXP", unit: "%"},
	"COOK_EXP_POINT_ADD":        {label: "Cooking EXP", unit: "%"},
	"ALCHEMY_EXP_POINT_ADD":     {label: "Alchemy EXP", unit: "%"},
	"HUNTING_EXP_POINT_ADD":     {label: "Hunting EXP", unit: "%"},
	"COLLECT_EXP_POINT_ADD":     {label: "Gathering Mastery"}, // CP tools: "increases Gathering Mastery", not EXP
	"MANUFACTURE_EXP_POINT_ADD": {label: "Processing EXP", unit: "%"},
	"VOYAGE_EXP_POINT_ADD":      {label: "Sailing EXP", unit: "%"},
	"QUEST_EXP":                 {label: "Quest EXP", unit: "%"},
	"EXP_POINT_ADD":             {label: "Combat EXP", unit: "%"},
	// Desc adds a scope caveat ("only affects Contribution EXP gained through
	// quests, excludes two specific weekly quests") but that's a footnote, not
	// part of the stat's name - keep the label plain like every other EXP entry.
	"CONTRIBUTION_EXP_ADD": {label: "Contribution EXP", unit: "%"},
	// Confirmed via description, not literally "Trading EXP": Trader's Clothes
	// grant extra chances at the Bargain minigame.
	"TRADING_EXP_POINT_ADD": {label: "Bargain Minigame Chances"},

	"MANUFACTURE_SUCC_UP":                {label: "Processing Success Rate", unit: "%"},
	"KNOWLEDGE_ACQUISITION_ADD":          {label: "Knowledge EXP", unit: "%"},
	"KNOWLEDGE_ACQUISITION_ADD_GRADE_UP": {label: "Knowledge EXP", unit: "%"},

	// Fishing-rod/gathering-tool stats.
	"SWIMMING_ENDURANCE_DOWN":         {label: "Swimming Stamina Consumption", unit: "%", negate: true},
	"ENERGY_FISH_REDUCE":              {label: "Fishing Energy Consumption", negate: true},
	"AUTO_FISHING_REDUCE_TIME_DOWN_2": {label: "Auto-Fishing Time", unit: "sec", negate: true},

	// Camp/siege/vehicle stats - confirmed real (mounts/ships/wagons/camps are
	// modeled as items too, so these do appear on Enhancement.Levels/Effects).
	"CAMP_WAREHOUSE_WEIGHT_ADD": {label: "Camp Storage Weight", unit: "LT"},
	"CANNON_DAM_CAL":            {label: "Cannon Damage"},
	"CANNON_DAM_CARAVEL":        {label: "Cannon Damage (Caravel)"},
	"CANNON_DAM_FRIGATE":        {label: "Cannon Damage (Frigate)"},
	"CANNON_ROLOADING_SPEED":    {label: "Cannon Reload Speed", unit: "%"},
	"BRAKE_ADD":                 {label: "Ship Brake"},
	"DAM_REDUCE_EFFECT":         {label: "Ship Damage Reduction", unit: "%"},
	"DEFENCEDOOR_DAM_ADD":       {label: "Damage to Siege Objects"},
	"DROP_THE_DAMAGE_DOWN":      {label: "Fall Damage", negate: true},
	"BOTTOM_TIME_ADD":           {label: "Underwater Breathing", unit: "sec"},
	"IN_SITE_FROM_ADD":          {label: "Vision Range"},

	// Mount stats - confirmed via Trainer's Clothes/riding crop tooltips.
	"BOARDING_EXP":                   {label: "Training Mastery"},
	"BOARDING_EXP_POINT_ACQUISITION": {label: "Mount EXP & Mount Skill EXP", unit: "%"},
	"HORSE_SKILL_EXP_UP":             {label: "Mount Skill EXP"},
	"HORSE_SKILL_UP":                 {label: "Mount Skill Learn Chance", unit: "%"},
	"TORQUE_ADD":                     {label: "Mount Skill Learn Chance", unit: "%"},
	"HARNESS_WHIP_EFFECT_1":          {label: "Training Mastery"},
	"HARNESS_WHIP_EFFECT_2":          {label: "Training Mastery"},
	"HARNESS_WHIP_EFFECT_3":          {label: "Training Mastery"},
}

// EffectNamedFuncs are value-less named effects (the data carries no number
// for these).
var EffectNamedFuncs = map[string]string{
	"ALL_AP_INCRE":             "All AP",
	"ALL_AP_INCRE_VALUE":       "All AP",
	"ALL_HIT_INCRE":            "All Accuracy",
	"ALL_DP_INCRE":             "All DP",
	"ALL_EVA_INCRE":            "All Evasion",
	"ALL_DAM_REDUCE_INCRE":     "All Damage Reduction",
	"MONSTER_DAM_ADD_INCRE":    "Extra AP Against Monsters Up",
	"MONSTER_DAM_ADD_INCRE_16": "Extra AP Against Monsters Up",
	"P_M_DAM_ADD_INCRE":        "Extra AP Against All Up",
	"P_M_DAM_ADD_INCRE_6":      "Extra AP Against All Up",
	"P_H_DAM_ADD_INCRE":        "Extra AP Against All Up",
	"MON_DAM_REDUCE_INCRE":     "Monster Damage Reduction Up",
	"MON_DAM_REDUCE_INCRE_16":  "Monster Damage Reduction Up",
	"AIN_DAM_ADD_INCRE":        "Extra AP Against Ahibs Up",

	"ALL_TRIBE_DAM_ADD_NOHUMAN_INCRE":          "Extra AP Against All (except Humans) Up",
	"HIDDEN_DAM_REDUCE_INCRE":                  "Damage Reduction (Hidden)",
	"HIDDEN_EVA_INCRE":                         "Evasion (Hidden)",
	"HP_RECOV_INCRE":                           "HP Recovery Up",
	"MAX_INDURANCE_INCRE":                      "Max Stamina Up",
	"JUMPING_INCRE":                            "Jump Height Up",
	"MANUFACTURE_SUCC_SLI_INCRE":               "Processing Success Rate Up",
	"CHANCE_LARGE_SPECIES_FISH_INCRE":          "Large Fish Chance Up",
	"CHANCE_RARE_SPECIES_FISH_INCRE":           "Rare Fish Chance Up",
	"SWIM_SPEED_INCRE_NO_1":                    "Swim Speed Up",
	"SWIM_SPEED_INCRE_NO_2":                    "Swim Speed Up",
	"SWIM_SPEED_INCRE_NO_3":                    "Swim Speed Up",
	"KNOWLEDGE_ACQUISITION_ADD_INCRE":          "Knowledge EXP Up",
	"KNOWLEDGE_ACQUISITION_ADD_INCRE_GRADE_UP": "Knowledge EXP Up",
	"AUTO_FISHING_REDUCE_TIME_DOWN_INCRE":      "Auto-Fishing Time Up",

	// Boolean/flavor capability flags - not a number worth formatting, so these
	// are named (valueless) even though a couple carry an unused arg.
	"ADD_SLOT_EQUIP_EFFECT":          "Vitclari Appearance",
	"BLACK_SPIRIT_FURY_INCRE":        "Black Spirit's Rage Max",
	"CAMP_REPAIR_USE":                "Camp Repair",
	"CAMP_SHOP_USE":                  "Camp Shop",
	"GUILD_WAR_COSTUME_EFFECT":       "Guild War Uniform Appearance",
	"VEHICLE_TRAINING_EXP_POINT_ADD": "Mount EXP & Taming Chance",
	"MINIGAMEFIND":                   "Lakiaro Digging",
	// Confirmed: a necklace "imbued with strong life energy, providing an
	// instant recovery" - a triggered set-effect, not a flat numeric stat.
	"ASADAL_EFFECT_1": "Instant HP Recovery",

	// Riding-crop gait bonuses (100+ item durability) - confirmed via tooltip,
	// but the gait itself (fast/run/wagon) isn't separately quantified.
	"HARNESS_WHIP_FAST_EFFECT_1":        "Horse Speed (Fast Gait)",
	"HARNESS_WHIP_FAST_EFFECT_2":        "Horse Speed (Fast Gait)",
	"HARNESS_WHIP_FAST_EFFECT_3":        "Horse Speed (Fast Gait)",
	"HARNESS_WHIP_RUN_EFFECT_1":         "Horse Speed (Running)",
	"HARNESS_WHIP_RUN_EFFECT_2":         "Horse Speed (Running)",
	"HARNESS_WHIP_RUN_EFFECT_3":         "Horse Speed (Running)",
	"HARNESS_WHIP_WAGON_EFFECT_1":       "Horse Speed (Wagon-Hitched)",
	"HARNESS_WHIP_WAGON_EFFECT_2":       "Horse Speed (Wagon-Hitched)",
	"HARNESS_WHIP_WAGON_EFFECT_3":       "Horse Speed (Wagon-Hitched)",
	"HARNESS_WHIP_WAGON_RUN_EFFECT_1":   "Horse Speed (Wagon Running)",
	"HARNESS_WHIP_WAGON_RUN_EFFECT_2":   "Horse Speed (Wagon Running)",
	"HARNESS_WHIP_WAGON_RUN_EFFECT_3":   "Horse Speed (Wagon Running)",
	"HARNESS_WHIP_WAGON_F_RUN_EFFECT_1": "Horse Speed (Wagon Fast Run)",
	"HARNESS_WHIP_WAGON_F_RUN_EFFECT_2": "Horse Speed (Wagon Fast Run)",
	"HARNESS_WHIP_WAGON_F_RUN_EFFECT_3": "Horse Speed (Wagon Fast Run)",

	// Sea Compass ship-speed bonuses (scaled by Sailing Mastery) - confirmed,
	// but the dash/run sub-mechanic itself isn't separately quantified.
	"SEA_COMPASS_SHIP_EFFECT_1":         "Ship Speed (Sailing Mastery)",
	"SEA_COMPASS_SHIP_EFFECT_2":         "Ship Speed (Sailing Mastery)",
	"SEA_COMPASS_SHIP_EFFECT_3":         "Ship Speed (Sailing Mastery)",
	"SEA_COMPASS_SHIP_DASH_EFFECT_1":    "Ship Dash Speed (Sailing Mastery)",
	"SEA_COMPASS_SHIP_DASH_EFFECT_2":    "Ship Dash Speed (Sailing Mastery)",
	"SEA_COMPASS_SHIP_DASH_EFFECT_3":    "Ship Dash Speed (Sailing Mastery)",
	"SEA_COMPASS_SHIP_RUN_EFFECT_1":     "Ship Run Speed (Sailing Mastery)",
	"SEA_COMPASS_SHIP_RUN_EFFECT_2":     "Ship Run Speed (Sailing Mastery)",
	"SEA_COMPASS_SHIP_RUN_EFFECT_3":     "Ship Run Speed (Sailing Mastery)",
	"SEA_COMPASS_SHIP_RUNDASH_EFFECT_1": "Ship Run-Dash Speed (Sailing Mastery)",
	"SEA_COMPASS_SHIP_RUNDASH_EFFECT_2": "Ship Run-Dash Speed (Sailing Mastery)",
	"SEA_COMPASS_SHIP_RUNDASH_EFFECT_3": "Ship Run-Dash Speed (Sailing Mastery)",

	// Stealth while gathering/squatting - confirmed, no numeric value.
	"COLLECT_POINT": "Stealth While Gathering",
}

// The per-accessory-grade set variant (ACCSET_1GRADE_LIFE_EXP_POINT_ADD - all
// grades confirmed "All Life Skill Mastery", same as the base
// LIFE_EXP_POINT_ADD), the per-gathering/processing-type mastery funcs
// (LIFESTAT_COLLECT_HOE_ADD, LIFESTAT_CRAFT_HEAT_ADD, ...), and the per-tool
// gathering drop-rate funcs (COLLECT_DROPRATE_DIG_ADD, ...) follow fixed
// prefixes; these fold them into their family stat instead of enumerating
// each one in EffectFuncs. (LIFE_EXP_N/LIFE_STAT_N/PO_LIFE_EXP_N do NOT follow
// this pattern - each numbered index is a distinct life skill, so those are
// enumerated explicitly in EffectFuncs instead.)
var (
	accSetLifeExpFuncPattern = regexp.MustCompile(`^ACCSET_\dGRADE_LIFE_EXP_POINT_ADD$`)
	lifestatCollectPattern   = regexp.MustCompile(`^LIFESTAT_COLLECT_\w+$`)
	lifestatCraftPattern     = regexp.MustCompile(`^LIFESTAT_CRAFT_\w+$`)
	collectDropratePattern   = regexp.MustCompile(`^COLLECT_DROPRATE(_\w+)?_ADD$`)
)

func FormatEffectFunctions(effects []EffectDsl, hasMarkers bool, groupTitle string) []EffectGroup {
	groups := make([]EffectGroup, 0)

	var current *EffectGroup = nil
	if !hasMarkers {
		current = &EffectGroup{Title: groupTitle}
	}

	for _, e := range effects {
		if title, ok := sectionMarkerFor(e.Func); ok {
			if current != nil {
				groups = append(groups, *current)
			}
			current = &EffectGroup{Title: title}
			continue
		}

		if current == nil {
			current = &EffectGroup{Title: "Effects"}
		}
		if current != nil {
			eff, _ := EffectFuncToStatMod(e)
			if current.Title == "Enhancement Effect" {
				if eff.Value == 0 && !strings.HasSuffix(eff.Stat, " Up") {
					eff.Stat = eff.Stat + " Up"
				}
			}

			current.Stats = append(current.Stats, eff)
		} else {
			log.Printf("WARNING: effect %q is not in a section, skipping", e.Func)
		}
	}

	if current != nil {
		groups = append(groups, *current)
	}

	return groups
}

func EffectFuncToStatMod(e EffectDsl) (StatMod, bool) {
	hasArg := len(e.Args) > 0
	arg := 0.0
	if hasArg {
		arg = e.Args[0]
	}

	if label, ok := EffectNamedFuncs[e.Func]; ok {
		return StatMod{
			EffectDsl: &e,
			Stat:      label,
			Value:     arg,
			Note:      "Boolean/Named Effect (no numeric value, or hard-coded in client)",
		}, true
	}

	if info, ok := ResolveEffectFunc(e.Func); ok {
		sign := "+"
		if info.negate {
			sign = "-"
		}
		m := StatMod{
			EffectDsl: &e,
			Stat:      info.label,
			Op:        sign,
			Negate:    info.negate,
			Value:     arg,
			Unit:      info.unit,
		}

		if !hasArg {
			m.Note = "Unknown (likely hard-coded stat value in client)"
			m.Op = "X"
		}

		return m, true
	}

	m := StatMod{
		EffectDsl: &EffectDsl{
			Func: e.Func,
			Args: e.Args,
		},
		Stat:  utils.HumanizeString(e.Func),
		Op:    "+",
		Value: arg,
	}
	if !hasArg {
		// fmt.Printf("WARNING: effect func %q is unmapped and has no arg\n", e.Func)
		m.Op = "X"
		m.Note = "Unmapped effect func with no arg"
	}

	return m, true
}

func ResolveEffectFunc(fn string) (EffectFuncInfo, bool) {
	if info, ok := EffectFuncs[fn]; ok {
		return info, true
	}
	switch {
	case accSetLifeExpFuncPattern.MatchString(fn):
		return EffectFuncInfo{label: "All Life Skill Mastery"}, true
	case lifestatCollectPattern.MatchString(fn):
		return EffectFuncInfo{label: "Gathering Mastery"}, true
	case lifestatCraftPattern.MatchString(fn):
		return EffectFuncInfo{label: "Processing Mastery"}, true
	case collectDropratePattern.MatchString(fn):
		return EffectFuncInfo{label: "Gathering Item Drop Rate", unit: "%"}, true
	}
	return EffectFuncInfo{}, false
}
