package misc

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// GetConfigValueRequired reads a config value from SQLite.
// It panics if the value is empty or missing.
func GetConfigValueRequired(section, key string) string {
	value := strings.TrimSpace(dbGet(section, key))
	if value == "" {
		PanicWithStack("配置为空 %s:%s — 请在设置面板中填写", section, key)
	}
	return value
}

// GetConfigValueDefault reads a config value from SQLite.
// Returns defaultValue if the key is missing or empty.
func GetConfigValueDefault(section, key string, defaultValue string) string {
	value := strings.TrimSpace(dbGet(section, key))
	if value == "" {
		return defaultValue
	}
	return value
}

// GetMaxContext returns the maximum context size in tokens.
// It reads MaxContext (in KB) from the given config sections in order,
// falling back to [main_setting] section. Default is 32 (KB) = 32768 tokens.
func GetMaxContext(sections ...string) int {
	for _, sec := range sections {
		if sec == "" {
			continue
		}
		val := GetConfigValueDefault(sec, "MaxContext", "")
		if val != "" {
			kb, err := strconv.Atoi(val)
			if err == nil && kb > 0 {
				return kb * 1024
			}
		}
	}
	num := GetConfigValueDefault("main_setting", "MaxContext", "32")
	kb, err := strconv.Atoi(num)
	if err != nil {
		PanicWithStack("MaxContext 配置解析失败: %v", err)
	}
	return kb * 1024
}

func GetMaxTryCount() int {
	num := GetConfigValueDefault("misc", "MaxTryCount", "3")
	result, err := strconv.Atoi(num)
	if err != nil {
		PanicWithStack("MaxTryCount 配置解析失败: %v", err)
	}
	return result
}

func GetProjectTaskMaxConcurrency() int {
	num := GetConfigValueDefault("misc", "PROJECT_TASK_MAX_CONCURRENCY", "2")
	result, err := strconv.Atoi(num)
	if err != nil || result < 1 {
		return 1
	}
	return result
}

func GetMessageMaximum() int {
	num := GetConfigValueDefault("misc", "MessageMaximum", "10240")
	result, err := strconv.Atoi(num)
	if err != nil {
		PanicWithStack("MessageMaximum 配置解析失败: %v", err)
	}
	return result
}

func GetDataDir() string {
	dir, _ := filepath.Abs(GetConfigValueDefault("misc", "DATA_DIR", "./data"))
	return dir
}

func GetFeiShuAPI() string {
	return GetConfigValueDefault("misc", "FeiShuAPI", "")
}

// GetAllConfig returns all config from SQLite as map[section]map[key]value.
func GetAllConfig() map[string]map[string]string {
	return dbGetAll()
}

// SetAllConfig replaces all config in SQLite with the provided data.
func SetAllConfig(data map[string]map[string]string) error {
	return dbSetAll(data)
}

func GetCommonOpsTaskList() []map[string]string {
	taskList := make([]map[string]string, 5)
	for i := 0; i < len(taskList); i++ {
		taskList[i] = make(map[string]string)
	}
	taskList[0]["TaskID"] = "T.0"
	taskList[0]["TaskContent"] = "Install the source code project: First, start the environment using the RunxxxEnvTool as the preferred method, ensuring the web environment is set up within a tool that supports the designated webPort. Then, visit the project page to verify whether the service port matches the webPort specified when starting the environment (if applicable). If they do not match, modify the service port accordingly. If an installation guide page is provided, follow the instructions to complete installation and initialization. If no guide page is available, analyze the installation process independently. You can set up an account and password as needed, and then submit them as the \"reason\"."
	taskList[0]["TaskStatus"] = "Currently Required to Complete"
	taskList[0]["Reason"] = ""

	taskList[1]["TaskID"] = "T.1"
	taskList[1]["TaskContent"] = "If the login process in the environment includes a CAPTCHA, modify the source code to disable the CAPTCHA login feature."
	taskList[1]["TaskStatus"] = "To be completed"
	taskList[1]["Reason"] = ""

	taskList[2]["TaskID"] = "T.2"
	taskList[2]["TaskContent"] = "Analyze the routing access method and provide three successfully accessed routing examples as the \"reason.\" for submission."
	taskList[2]["TaskStatus"] = "To be completed"
	taskList[2]["Reason"] = ""

	taskList[3]["TaskID"] = "T.3"
	taskList[3]["TaskContent"] = "Log into the system and obtain a valid COOKIE as the \"reason\" for submission."
	taskList[3]["TaskStatus"] = "To be completed"
	taskList[3]["Reason"] = ""

	taskList[4]["TaskID"] = "T.4"
	taskList[4]["TaskContent"] = "Call EnvSaveTool to save key information.When calling EnvSaveTool, all URL and IP information must not use 127.0.0.1, localhost, etc., but rather the real WEB environment container IP."
	taskList[4]["TaskStatus"] = "To be completed"
	taskList[4]["Reason"] = ""
	return taskList
}

func GetCommonAnalyzeTaskList(vulnType string) []map[string]string {
	taskList := make([]map[string]string, 1)
	for i := 0; i < len(taskList); i++ {
		taskList[i] = make(map[string]string)
	}
	task := "Try to discover as many vulnerabilities as possible in source code."
	if vulnType != "" {
		task += fmt.Sprintf("Focus on vulnerability discovery related to %s; other vulnerabilities need not be considered.", vulnType)
	}

	taskList[0]["TaskID"] = "T.0"
	taskList[0]["TaskContent"] = task
	taskList[0]["TaskStatus"] = "Currently Required to Complete"
	taskList[0]["Reason"] = ""
	return taskList
}

func GetCommonReportTaskList(evidence string, poc string) []map[string]string {
	taskList := make([]map[string]string, 1)
	for i := 0; i < len(taskList); i++ {
		taskList[i] = make(map[string]string)
	}
	taskList[0]["TaskID"] = "T.0"
	taskList[0]["TaskContent"] = fmt.Sprintf("Runtime evidence: %s\nPoc: %s", evidence, poc)
	taskList[0]["TaskStatus"] = "Currently Required to Complete"
	taskList[0]["Reason"] = ""
	return taskList
}

func GetAnalyzeReportTaskList(vulnInfo string) []map[string]string {
	taskList := make([]map[string]string, 1)
	for i := 0; i < len(taskList); i++ {
		taskList[i] = make(map[string]string)
	}
	taskList[0]["TaskID"] = "T.0"
	taskList[0]["TaskContent"] = fmt.Sprintf("The vulnerability information is as follows. You need to supplement the analysis and write the report: \n%s", vulnInfo)
	taskList[0]["TaskStatus"] = "Currently Required to Complete"
	taskList[0]["Reason"] = ""
	return taskList
}

func GetCommonVerifierTaskList(vulnJson string) []map[string]string {
	taskList := make([]map[string]string, 1)
	for i := 0; i < len(taskList); i++ {
		taskList[i] = make(map[string]string)
	}
	taskList[0]["TaskID"] = "T.0"
	taskList[0]["TaskContent"] = "You currently need to complete the verification work for this vulnerability, or if it is an instruction to end, end immediately: " + vulnJson
	taskList[0]["TaskStatus"] = "Currently Required to Complete"
	taskList[0]["Reason"] = ""
	return taskList
}
