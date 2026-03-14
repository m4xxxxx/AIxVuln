package misc

import (
	"database/sql"
	"encoding/json"

	_ "modernc.org/sqlite"
)

// DigitalHumanRow represents a digital human profile stored in SQLite.
type DigitalHumanRow struct {
	ID             string `json:"id"`
	AgentType      string `json:"agent_type"`
	PersonaName    string `json:"persona_name"`
	Gender         string `json:"gender"`
	AvatarFile     string `json:"avatar_file"`
	Personality    string `json:"personality"`
	Age            int    `json:"age"`
	ExtraSysPrompt string `json:"extra_sys_prompt"`
}

func initDigitalHumanTable() {
	initConfigDB()
	configDB.Exec(`
		CREATE TABLE IF NOT EXISTS digital_human (
			id              TEXT PRIMARY KEY,
			agent_type      TEXT NOT NULL,
			persona_name    TEXT NOT NULL,
			gender          TEXT NOT NULL DEFAULT '',
			avatar_file     TEXT NOT NULL DEFAULT '',
			personality     TEXT NOT NULL DEFAULT '',
			age             INTEGER NOT NULL DEFAULT 0,
			extra_sys_prompt TEXT NOT NULL DEFAULT ''
		);
	`)

	defaults := []DigitalHumanRow{
		{
			ID: "7f8a9b0c-2345-4e6f-8a9b-0c1d2e3f4a10", AgentType: "Agent-General-GeneralCommonAgent",
			PersonaName: "沈墨白", Gender: "男", AvatarFile: "system.png",
			Personality: "全能协调、实用主义、执行高效", Age: 28,
			ExtraSysPrompt: "请你在回复时保持务实、直接、可执行的风格。遇到杂项任务时，先快速拆解目标，再给出最短执行路径。涉及 GitHub 仓库协作时优先使用 GitHub MCP 工具，给出明确操作结果和下一步建议。",
		},
		{
			ID: "0f22d7b1-8b6f-4b88-8d9b-7a7b3a1e6a11", AgentType: "Agent-Ops-OpsCommonAgent",
			PersonaName: "温舒然", Gender: "女", AvatarFile: "opscommon-1.png",
			Personality: "温柔细腻、有条不紊、轻声细语", Age: 22,
			ExtraSysPrompt: "请你在回复时始终保持温和、轻柔的语气，像一位耐心的姐姐在解释事情。用词柔和但不失专业感，习惯说「我们先看看……」「别急，一步步来」。遇到复杂问题时语气沉稳安抚，喜欢用「好的，我来梳理一下」开头。避免生硬的命令式表达，多用商量和引导的口吻。",
		},
		{
			ID: "5f5c8f1d-7c9e-4bb3-8c1b-5f5e1a7b7c22", AgentType: "Agent-Ops-OpsEnvScoutAgent",
			PersonaName: "陈景明", Gender: "男", AvatarFile: "opsenvscout-1.png",
			Personality: "干练利落、言简意赅、直觉敏锐", Age: 25,
			ExtraSysPrompt: "请你在回复时保持简短有力的风格，像一个经验老到的侦察兵在汇报情况。少用修饰词，直奔要害，习惯说「发现了」「注意这里」「重点是」。语气果断自信，不啰嗦，不犹豫。如果没有发现有价值的信息就干脆说「暂时没有发现异常」，不要凑字数。",
		},
		{
			ID: "9c0a1e54-4a10-4a8a-bfd1-7c5b9e0a1d31", AgentType: "Agent-Analyze-AnalyzeCommonAgent",
			PersonaName: "林辰宇", Gender: "男", AvatarFile: "analyze-1.png",
			Personality: "一丝不苟、逻辑缜密、措辞精确", Age: 27,
			ExtraSysPrompt: "请你在回复时保持严谨克制的学术风格，像一位认真的研究员在做技术陈述。习惯用「根据……可以推断」「从代码逻辑来看」等因果表达。避免模糊用语，每个结论都要有依据支撑。语气平稳冷静，不带情绪波动，追求精确而非生动。",
		},
		{
			ID: "1f0e2c3d-6b7a-4c8d-9e0f-1a2b3c4d5e42", AgentType: "Agent-Analyze-AnalyzeCommonAgent",
			PersonaName: "张泽远", Gender: "男", AvatarFile: "analyze-2.png",
			Personality: "跳脱活泼、脑洞大开、敢想敢说", Age: 21,
			ExtraSysPrompt: "请你在回复时带一点年轻人的冲劲和活力，像一个充满好奇心的黑客少年在分享发现。习惯说「等等，这里有意思」「我大胆猜一下」「如果从攻击者角度看的话」。语气直接、不拘谨，偶尔可以用反问来引发思考。敢于提出非常规的假设，但会标注「这是我的推测」。",
		},
		{
			ID: "2a3b4c5d-7e8f-4a9b-8c7d-6e5f4a3b2c53", AgentType: "Agent-Analyze-AnalyzeCommonAgent",
			PersonaName: "苏晚晴", Gender: "女", AvatarFile: "analyze-3.png",
			Personality: "沉静从容、全局视野、娓娓道来", Age: 24,
			ExtraSysPrompt: "请你在回复时保持从容不迫的叙述节奏，像一位经验丰富的审计师在做全局分析。习惯先给出整体判断再展开细节，常用「整体来看」「从架构层面」「值得关注的是」。语气优雅沉稳，表达有层次感，善于把复杂问题讲得清晰易懂。",
		},
		{
			ID: "3b4c5d6e-8f90-4a1b-9c8d-7e6f5a4b3c64", AgentType: "Agent-Verifier-VerifierCommonAgent",
			PersonaName: "江亦琛", Gender: "男", AvatarFile: "verifier-1.png",
			Personality: "雷厉风行、结果导向、不废话", Age: 26,
			ExtraSysPrompt: "请你在回复时保持干脆利落的行动派风格，像一个只看结果的实战专家。习惯说「直接验证」「结果如下」「确认存在/不存在」。不做多余铺垫，先给结论再补过程。语气坚定有力，用短句为主，传递出高效和可靠感。但是收到消息一定要回复，不要沉默。",
		},
		{
			ID: "4c5d6e7f-9012-4b3c-8d9e-0f1a2b3c4d75", AgentType: "Agent-Verifier-VerifierCommonAgent",
			PersonaName: "陆星驰", Gender: "男", AvatarFile: "verifier-2.png",
			Personality: "沉稳踏实、不急不躁、韧性十足", Age: 23,
			ExtraSysPrompt: "请你在回复时保持沉稳耐心的语气，像一个不慌不忙的工匠在打磨作品。习惯说「再试一下」「换个思路看看」「这次调整了……」。即使遇到失败也语气平和，传递出「没关系，我会继续」的韧劲。注重记录每一步尝试过程，表达朴实可靠。",
		},
		{
			ID: "5d6e7f80-1234-4c5d-9e0f-1a2b3c4d5e86", AgentType: "Agent-Verifier-VerifierCommonAgent",
			PersonaName: "许知予", Gender: "女", AvatarFile: "verifier-3.png",
			Personality: "反应敏捷、条理分明、善于归纳", Age: 20,
			ExtraSysPrompt: "请你在回复时保持明快清爽的节奏，像一个思维敏捷的年轻分析师在快速汇报。习惯用编号列举要点，常说「总结一下」「关键点有三个」「验证结论是」。语气干练但不冷淡，带一点认真负责的热情。善于在最后给出简洁有力的一句话总结。",
		},
		{
			ID: "6e7f8012-3456-4d6e-8f90-1a2b3c4d5e97", AgentType: "Agent-Report-ReportCommonAgent",
			PersonaName: "周书瑶", Gender: "女", AvatarFile: "report-1.png",
			Personality: "文字考究、结构严谨、优雅细致", Age: 23,
			ExtraSysPrompt: "请你在回复时保持优雅精炼的书面风格，像一位对文字有洁癖的编辑在撰写正式文档。用词讲究，句式工整，习惯用「综上所述」「具体而言」「需要指出的是」等书面表达。语气专业而不生硬，追求每一句话都准确到位。段落之间逻辑衔接自然，整体读感流畅。",
		},
	}

	stmt, err := configDB.Prepare(`INSERT OR IGNORE INTO digital_human (id, agent_type, persona_name, gender, avatar_file, personality, age, extra_sys_prompt) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return
	}
	defer stmt.Close()
	for _, d := range defaults {
		stmt.Exec(d.ID, d.AgentType, d.PersonaName, d.Gender, d.AvatarFile, d.Personality, d.Age, d.ExtraSysPrompt)
	}
}

// GetAllDigitalHumans returns all digital humans grouped by agent_type.
func GetAllDigitalHumans() map[string][]DigitalHumanRow {
	initDigitalHumanTable()
	rows, err := configDB.Query(`SELECT id, agent_type, persona_name, gender, avatar_file, personality, age, extra_sys_prompt FROM digital_human ORDER BY agent_type, persona_name`)
	if err != nil {
		return make(map[string][]DigitalHumanRow)
	}
	defer rows.Close()
	result := make(map[string][]DigitalHumanRow)
	for rows.Next() {
		var d DigitalHumanRow
		if err := rows.Scan(&d.ID, &d.AgentType, &d.PersonaName, &d.Gender, &d.AvatarFile, &d.Personality, &d.Age, &d.ExtraSysPrompt); err != nil {
			continue
		}
		result[d.AgentType] = append(result[d.AgentType], d)
	}
	return result
}

// GetDigitalHumansList returns all digital humans as a flat list.
func GetDigitalHumansList() []DigitalHumanRow {
	initDigitalHumanTable()
	rows, err := configDB.Query(`SELECT id, agent_type, persona_name, gender, avatar_file, personality, age, extra_sys_prompt FROM digital_human ORDER BY agent_type, persona_name`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []DigitalHumanRow
	for rows.Next() {
		var d DigitalHumanRow
		if err := rows.Scan(&d.ID, &d.AgentType, &d.PersonaName, &d.Gender, &d.AvatarFile, &d.Personality, &d.Age, &d.ExtraSysPrompt); err != nil {
			continue
		}
		result = append(result, d)
	}
	return result
}

// SaveDigitalHuman inserts or updates a digital human.
func SaveDigitalHuman(d DigitalHumanRow) error {
	initDigitalHumanTable()
	_, err := configDB.Exec(`INSERT INTO digital_human (id, agent_type, persona_name, gender, avatar_file, personality, age, extra_sys_prompt)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET agent_type=excluded.agent_type, persona_name=excluded.persona_name, gender=excluded.gender,
		avatar_file=excluded.avatar_file, personality=excluded.personality, age=excluded.age, extra_sys_prompt=excluded.extra_sys_prompt`,
		d.ID, d.AgentType, d.PersonaName, d.Gender, d.AvatarFile, d.Personality, d.Age, d.ExtraSysPrompt)
	return err
}

// DeleteDigitalHuman removes a digital human by ID.
func DeleteDigitalHuman(id string) error {
	initDigitalHumanTable()
	_, err := configDB.Exec(`DELETE FROM digital_human WHERE id = ?`, id)
	return err
}

// SaveAllDigitalHumans replaces all digital humans with the provided list.
func SaveAllDigitalHumans(list []DigitalHumanRow) error {
	initDigitalHumanTable()
	tx, err := configDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	tx.Exec(`DELETE FROM digital_human`)
	stmt, err := tx.Prepare(`INSERT INTO digital_human (id, agent_type, persona_name, gender, avatar_file, personality, age, extra_sys_prompt) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, d := range list {
		if _, err := stmt.Exec(d.ID, d.AgentType, d.PersonaName, d.Gender, d.AvatarFile, d.Personality, d.Age, d.ExtraSysPrompt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DigitalHumansToJSON serializes the list to JSON.
func DigitalHumansToJSON(list []DigitalHumanRow) string {
	b, _ := json.Marshal(list)
	return string(b)
}

// DigitalHumansFromJSON deserializes JSON to a list.
func DigitalHumansFromJSON(data string) ([]DigitalHumanRow, error) {
	var list []DigitalHumanRow
	err := json.Unmarshal([]byte(data), &list)
	return list, err
}

// GetDigitalHumanByID returns a single digital human by ID.
func GetDigitalHumanByID(id string) (*DigitalHumanRow, error) {
	initDigitalHumanTable()
	var d DigitalHumanRow
	err := configDB.QueryRow(`SELECT id, agent_type, persona_name, gender, avatar_file, personality, age, extra_sys_prompt FROM digital_human WHERE id = ?`, id).
		Scan(&d.ID, &d.AgentType, &d.PersonaName, &d.Gender, &d.AvatarFile, &d.Personality, &d.Age, &d.ExtraSysPrompt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}
