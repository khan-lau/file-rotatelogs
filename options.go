package rotatelogs

import (
	"time"

	"github.com/khan-lau/file-rotatelogs/internal/option"
)

const (
	optkeyClock         = "clock"
	optkeyHandler       = "handler"
	optkeyLinkName      = "link-name"
	optkeyMaxAge        = "max-age"
	optkeyRotationTime  = "rotation-time"
	optkeyRotationSize  = "rotation-size"
	optkeyRotationCount = "rotation-count"
	optkeyForceNewFile  = "force-new-file"
	optkeyNamingFunc    = "naming-func" // 自定义滚动命名规则
	optkeyAgingFunc     = "aging-func"  // 自定义老化清理逻辑
)

// WithClock creates a new Option that sets a clock
// that the RotateLogs object will use to determine
// the current time.
//
// By default rotatelogs.Local, which returns the
// current time in the local time zone, is used. If you
// would rather use UTC, use rotatelogs.UTC as the argument
// to this option, and pass it to the constructor.
func WithClock(c Clock) Option {
	return option.New(optkeyClock, c)
}

// WithLocation creates a new Option that sets up a
// "Clock" interface that the RotateLogs object will use
// to determine the current time.
//
// This optin works by always returning the in the given
// location.
func WithLocation(loc *time.Location) Option {
	return option.New(optkeyClock, clockFn(func() time.Time {
		return time.Now().In(loc)
	}))
}

// WithLinkName creates a new Option that sets the
// symbolic link name that gets linked to the current
// file name being used.
func WithLinkName(s string) Option {
	return option.New(optkeyLinkName, s)
}

// WithMaxAge creates a new Option that sets the
// max age of a log file before it gets purged from
// the file system.
func WithMaxAge(d time.Duration) Option {
	return option.New(optkeyMaxAge, d)
}

// WithRotationTime creates a new Option that sets the
// time between rotation.
func WithRotationTime(d time.Duration) Option {
	return option.New(optkeyRotationTime, d)
}

// WithRotationSize creates a new Option that sets the
// log file size between rotation.
func WithRotationSize(s int64) Option {
	return option.New(optkeyRotationSize, s)
}

// WithRotationCount creates a new Option that sets the
// number of files should be kept before it gets
// purged from the file system.
func WithRotationCount(n uint) Option {
	return option.New(optkeyRotationCount, n)
}

// WithHandler creates a new Option that specifies the
// Handler object that gets invoked when an event occurs.
// Currently `FileRotated` event is supported
func WithHandler(h Handler) Option {
	return option.New(optkeyHandler, h)
}

// ForceNewFile ensures a new file is created every time New()
// is called. If the base file name already exists, an implicit
// rotation is performed
func ForceNewFile() Option {
	return option.New(optkeyForceNewFile, true)
}

// WithNamingFunc 设置自定义滚动日志文件命名规则。
// 当需要滚动日志且新文件名与已有文件冲突时，回调此函数。
// 参数为 strftime 模式生成的基础文件名和代际编号，返回代际文件名。
//
// 不设置时默认规则为："文件名.代际编号.扩展名"（如 app.log → app.1.log）。
func WithNamingFunc(f NamingFunc) Option {
	return option.New(optkeyNamingFunc, f)
}

// WithAgingFunc 设置自定义日志老化清理回调，决定哪些旧日志文件应被删除。
// 设置后将完全替代内置的 maxAge / rotationCount 清理逻辑。
//
// 回调接收所有匹配的日志文件（含 os.FileInfo），
// 返回要删除的文件路径列表。
func WithAgingFunc(f AgingFunc) Option {
	return option.New(optkeyAgingFunc, f)
}
