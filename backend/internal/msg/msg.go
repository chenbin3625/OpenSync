package msg

import "fmt"

const (
	LostPart                 = "入参不全"
	SignIn                   = "请登录"
	LoginExpired             = "登录失效"
	AlistNotFound            = "未找到alist，可能已经被删除"
	AlistInUse               = "该引擎仍被同步任务使用，请先删除相关同步任务"
	JobNotFound              = "未找到作业，可能已经被删除"
	TaskNotFound             = "未找到任务，可能已经被删除"
	UserNotFound             = "用户不存在"
	SystemInitialized        = "系统已完成初始化"
	AlistConnectFail         = "alist连接失败，请检查是否填写正确"
	AlistURLInvalid          = "AList地址必须是有效的 http 或 https URL"
	AddressIncorrect         = "alist地址格式有误"
	CodeNot200               = "状态码非200"
	AlistUnAuth              = "AList鉴权失败，可能是令牌已失效"
	WithoutToken             = "地址改变时令牌必填"
	TaskMayDelete            = "任务未找到。可能是您手动到AList中删除了复制任务；或者Alist因手动/异常奔溃被重启，导致任务记录丢失"
	Src                      = "来源"
	Dst                      = "目标"
	NoJobForRun              = "没有可供执行的作业"
	JobRunning               = "当前有任务执行中，请稍后再试"
	JobRunningCannotDelete   = "当前同步任务正在执行中，不能删除"
	JobDeleteWaitTimeout     = "任务仍在停止中，请稍后重试删除"
	IntervalLost             = "创建间隔型作业时，间隔必填"
	CronLost                 = "创建cron型任务时，至少有一项不为空"
	CannotResumeLostJob      = "作业不存在无法恢复，请删除后重新创建"
	DisabledJobCannotRun     = "禁用的作业不能运行"
	CannotDisableManualJob   = "不可禁用仅手动任务"
	TaskNotRunningStop       = "任务未在运行中，无法停止"
	NoFailedTaskItems        = "没有可重试的未完成项"
	PasswdWrong              = "密码错误"
	KeyWrong                 = "恢复密钥错误"
	PasswdWrongMaxTime       = "5分钟内密码错误超过3次，请稍后再试"
	NotifyTestMsg            = "这是一条由您自己发送的OpenSync测试消息，当你看到这条消息，说明你的配置是正确可用的。"
	NotifyURLInvalid         = "Webhook URL必须是有效的HTTPS地址"
	NotifySendFail           = "通知发送失败，请检查配置或稍后重试"
	PasswdTooShort           = "密码长度至少需要8位"
	PasswdTooLong            = "密码长度不能超过72字节"
	MinFileSizeInvalid       = "最小文件大小必须是大于等于0的整数"
	MaxFileSizeInvalid       = "最大文件大小必须是大于等于0的整数"
	MinFileSizeGtMax         = "最小文件大小不能大于最大文件大小"
	SettingsExpires          = "登录有效期"
	SettingsTaskTimeout      = "任务超时时间"
	SettingsTaskSave         = "历史任务保留"
	SettingsCopyConcurrency  = "复制并发数"
	SettingsScanConcurrency  = "扫描并发数"
	SettingsMaxRetries       = "最大重试次数"
)

func ScanError(srcOrDst, reason string) string {
	return fmt.Sprintf("%s目录扫描失败，原因为: %s", srcOrDst, reason)
}

func AlistFailCodeReason(code int, message string) string {
	return fmt.Sprintf("AList返回%d错误，原因为：%s", code, message)
}

func NotifyError(reason string) string {
	return fmt.Sprintf("发送通知过程中失败，原因为：%s", reason)
}

func SettingsRangeError(name string, min, max int) string {
	return fmt.Sprintf("%s必须在%d到%d之间", name, min, max)
}
