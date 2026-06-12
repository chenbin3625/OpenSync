package i18n

import (
	"fmt"
	"opensync/pkg/crypto"
	"sync"
)

var sysLanguage string
var sysLanguageMu sync.RWMutex

var allLng = map[string]map[string]interface{}{
	"zh_cn": {
		"success":                          "操作成功",
		"lost_part":                        "入参不全",
		"same_exists":                      "已存在相同数据，请检查！",
		"sign_in":                          "请登录",
		"login_expired":                    "登录失效",
		"alist_not_found":                  "未找到alist，可能已经被删除",
		"alist_in_use":                     "该引擎仍被同步任务使用，请先删除相关同步任务",
		"del_job_course_error":             "作业启动过程中报错，将自动删除作业，任务为 {}",
		"job_not_found":                    "未找到作业，可能已经被删除",
		"task_not_found":                   "未找到任务，可能已经被删除",
		"user_not_found":                   "用户不存在",
		"alist_connect_fail":               "alist连接失败，请检查是否填写正确",
		"address_incorrect":                "alist地址格式有误",
		"code_not_200":                     "状态码非200",
		"alist_un_auth":                    "AList鉴权失败，可能是令牌已失效",
		"alist_fail_code_reason":           "AList返回{}错误，原因为：{}",
		"without_token":                    "地址改变时令牌必填",
		"add_alist_client_fail":            "新增alist客户端时失败，原因为：{}",
		"task_may_delete":                  "任务未找到。可能是您手动到AList中删除了复制任务；或者Alist因手动/异常奔溃被重启，导致任务记录丢失",
		"do_job_err":                       "执行任务失败，原因为：{}",
		"src":                              "来源",
		"dst":                              "目标",
		"scan_error":                       "{}目录扫描失败，原因为: {}",
		"copy_success_but_delete_fail":     "文件复制成功，但删除源文件失败，删除失败的原因是：{}",
		"no_job_for_run":                   "没有可供执行的作业",
		"job_running":                      "当前有任务执行中，请稍后再试",
		"job_running_cannot_delete":        "当前同步任务正在执行中，不能删除",
		"job_delete_wait_timeout":          "任务仍在停止中，请稍后重试删除",
		"interval_lost":                    "创建间隔型作业时，间隔必填",
		"cron_lost":                        "创建cron型任务时，至少有一项不为空",
		"cannot_resume_lost_job":           "作业不存在无法恢复，请删除后重新创建",
		"stop_fail":                        "停止定时任务失败，原因为：{}",
		"disable_fail":                     "禁用定时任务失败，原因为：{}",
		"cancel_fail":                      "取消任务过程中失败，原因为：{}",
		"disable_then_edit":                "请先禁用任务才能编辑",
		"disabled_job_cannot_run":          "禁用的作业不能运行",
		"cannot_disable_manual_job":        "不可禁用仅手动任务",
		"task_not_running":                 "任务未在运行中，无法暂停",
		"no_failed_task_items":             "没有失败项可重试",
		"no_resumable_task_items":          "没有可继续执行的中断项",
		"log_del_success":                  "日志文件{}已被成功删除",
		"log_del_fail":                     "日志文件{}删除失败，原因为：{}",
		"log_rename_start":                 "日志定时更名任务启动成功",
		"keep_all_log":                     "日志保留时间为0，将保留所有日志",
		"keep_all_task":                    "任务保留时间为0，将保留所有任务",
		"clear_task_start":                 "定时清理任务启动成功",
		"passwd_wrong":                     "密码错误",
		"key_wrong":                        "加密秘钥错误",
		"passwd_wrong_max_time":            "5分钟内密码错误超过3次，请稍后再试",
		"notify_error":                     "发送通知过程中失败，原因为：{}",
		"notify_test_msg":                  "这是一条由您自己发送的OpenSync测试消息，当你看到这条消息，说明你的配置是正确可用的。",
		"min_file_size_invalid":            "最小文件大小必须是大于等于0的整数",
		"max_file_size_invalid":            "最大文件大小必须是大于等于0的整数",
		"min_file_size_gt_max":             "最小文件大小不能大于最大文件大小",
		"settings_expires":                 "登录有效期",
		"settings_task_timeout":            "任务超时时间",
		"settings_task_save":               "历史任务保留",
		"settings_copy_concurrency":        "复制并发数",
		"settings_scan_concurrency":        "扫描并发数",
		"settings_realtime_finished_items": "完成明细保留数",
		"settings_max_retries":             "最大重试次数",
		"settings_range_error":             "%s必须在%d到%d之间",
	},
	"eng": {
		"success":                          "success",
		"lost_part":                        "Incomplete participation",
		"same_exists":                      "The same data already exists, please check!",
		"sign_in":                          "Please sign in",
		"login_expired":                    "Login expired",
		"alist_not_found":                  "Alist not found, may have been deleted",
		"alist_in_use":                     "This engine is still used by sync jobs. Delete related jobs first.",
		"del_job_course_error":             "An error occurred during job startup and the job will be automatically deleted. The job is {}",
		"job_not_found":                    "The job was not found and may have been deleted",
		"task_not_found":                   "Task not found, may have been deleted",
		"user_not_found":                   "User does not exist",
		"alist_connect_fail":               "Alist connection failed, please check whether it is filled in correctly",
		"address_incorrect":                "The alist address format is incorrect",
		"code_not_200":                     "Code not 200",
		"alist_un_auth":                    "AList authentication failed, possibly because the token has expired",
		"alist_fail_code_reason":           "AList returns {}, reason: {}",
		"without_token":                    "Token is required when address changes",
		"add_alist_client_fail":            "Failed to add alist client, reason: {}",
		"task_may_delete":                  "task not found. You may have manually deleted the replication task in AList; or Alist was restarted manually or abnormally, resulting in the loss of task records",
		"do_job_err":                       "Task execution failed due to: {}",
		"src":                              "source",
		"dst":                              "target",
		"scan_error":                       "{} directory scan failed due to: {}",
		"copy_success_but_delete_fail":     "The file was copied successfully, but the source file failed to be deleted due to: {}",
		"no_job_for_run":                   "No jobs available to execute",
		"job_running":                      "There is a task currently being executed, please try again later",
		"job_running_cannot_delete":        "This sync job is currently running and cannot be deleted.",
		"job_delete_wait_timeout":          "The job is still stopping. Please retry deletion later.",
		"interval_lost":                    "When creating an interval job, the interval is required",
		"cron_lost":                        "When creating a cron job, at least one of the following items must be non-empty",
		"cannot_resume_lost_job":           "The job does not exist and cannot be restored. Please delete it and create it again",
		"stop_fail":                        "Failed to stop the scheduled task due to: {}",
		"disable_fail":                     "Failed to pause the scheduled task due to: {}",
		"cancel_fail":                      "The task cancellation process failed due to: {}",
		"disable_then_edit":                "Please disable the job before editing it",
		"disabled_job_cannot_run":          "Disabled jobs cannot be run.",
		"cannot_disable_manual_job":        "Manual-only jobs cannot be disabled",
		"task_not_running":                 "The task is not running and cannot be paused",
		"no_failed_task_items":             "No failed task items to retry",
		"no_resumable_task_items":          "No interrupted task items to resume",
		"log_del_success":                  "The log file {} has been successfully deleted",
		"log_del_fail":                     "Failed to delete log file {}, reason: {}",
		"log_rename_start":                 "The log scheduled renaming task was started successfully",
		"keep_all_log":                     "The log retention time is 0, all logs will be retained",
		"keep_all_task":                    "The task retention time is 0, all tasks will be retained",
		"clear_task_start":                 "The scheduled cleanup task was started successfully",
		"passwd_wrong":                     "Wrong password",
		"key_wrong":                        "Wrong key",
		"passwd_wrong_max_time":            "The password was incorrect more than 3 times within 5 minutes. Please try again later",
		"notify_error":                     "Failed to send notification due to: {}",
		"notify_test_msg":                  "This is a OpenSync test message sent by yourself. When you see this message, it means your configuration is correct and available.",
		"min_file_size_invalid":            "Minimum file size must be an integer greater than or equal to 0",
		"max_file_size_invalid":            "Maximum file size must be an integer greater than or equal to 0",
		"min_file_size_gt_max":             "Minimum file size cannot be greater than maximum file size",
		"settings_expires":                 "Login validity period",
		"settings_task_timeout":            "Task timeout",
		"settings_task_save":               "Historical task retention",
		"settings_copy_concurrency":        "Copy concurrency",
		"settings_scan_concurrency":        "Scan concurrency",
		"settings_realtime_finished_items": "Completed detail retention",
		"settings_max_retries":             "Maximum retries",
		"settings_range_error":             "%s must be between %d and %d",
	},
}

// GetLanguage returns current language
func GetLanguage() string {
	sysLanguageMu.RLock()
	lang := sysLanguage
	sysLanguageMu.RUnlock()
	if lang != "" {
		return lang
	}

	sysLanguageMu.Lock()
	defer sysLanguageMu.Unlock()
	if sysLanguage == "" {
		sysLanguage = crypto.ReadOrSetFile("data/language.txt", "zh_cn", false)
	}
	return sysLanguage
}

// SetLanguage sets the language
func SetLanguage(lang string) error {
	if _, ok := allLng[lang]; !ok {
		return Errorf("no %s", lang)
	}
	sysLanguageMu.Lock()
	sysLanguage = lang
	sysLanguageMu.Unlock()
	crypto.ReadOrSetFile("data/language.txt", lang, true)
	return nil
}

// G returns the translated string for given key
func G(key string) string {
	lang := GetLanguage()
	if msgs, ok := allLng[lang]; ok {
		if v, ok := msgs[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	// Fallback to zh_cn
	if msgs, ok := allLng["zh_cn"]; ok {
		if v, ok := msgs[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return key
}

// Errorf creates a simple error (helper)
type simpleError struct{ msg string }

func (e *simpleError) Error() string { return e.msg }
func Errorf(format string, args ...interface{}) error {
	return &simpleError{msg: fmt.Sprintf(format, args...)}
}
