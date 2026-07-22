package rotatelogs

import (
	"os"
	"sync"
	"time"

	strftime "github.com/lestrrat-go/strftime"
)

type Handler interface {
	Handle(Event)
}

type HandlerFunc func(Event)

type Event interface {
	Type() EventType
}

type EventType int

const (
	InvalidEventType EventType = iota
	FileRotatedEventType
)

type FileRotatedEvent struct {
	prev    string // previous filename
	current string // current, new filename
}

// NamingFunc 自定义滚动日志文件命名规则
//   - baseFilename: strftime 模式生成的基础文件名（如 "/var/log/app.log"）
//   - generation: 代际编号，从 1 开始递增
//
// 返回值为该代际实际使用的完整文件名
// 若不设置，默认规则为 "文件名.代际.扩展名"（如 app.1.log）
type NamingFunc func(baseFilename string, generation int) string

// LogFileInfo 记录匹配到的日志文件路径及其文件信息，
// 供 AgingFunc 自定义老化清理逻辑使用
type LogFileInfo struct {
	Path     string      // 文件完整路径
	FileInfo os.FileInfo // 文件元信息（ModTime、Size 等）
}

// AgingFunc 自定义日志老化清理回调，决定哪些旧日志文件应被删除
//   - files: 所有匹配的日志文件（已过滤 _lock/_symlink）
//
// 返回值为要删除的文件路径列表
// 设置后将完全替代内置的 maxAge / rotationCount 清理逻辑
type AgingFunc func(files []LogFileInfo) []string

// RotateLogs represents a log file that gets
// automatically rotated as you write to it.
type RotateLogs struct {
	clock         Clock
	curFn         string
	curBaseFn     string
	globPattern   string
	generation    int
	linkName      string
	maxAge        time.Duration
	mutex         sync.RWMutex
	eventHandler  Handler
	outFh         *os.File
	pattern       *strftime.Strftime
	rotationTime  time.Duration
	rotationSize  int64
	rotationCount uint
	forceNewFile  bool       // 强制新建文件（忽略现有文件）
	namingFunc    NamingFunc // 自定义滚动命名规则回调
	agingFunc     AgingFunc  // 自定义老化清理回调
}

// Clock is the interface used by the RotateLogs
// object to determine the current time
type Clock interface {
	Now() time.Time
}
type clockFn func() time.Time

// UTC is an object satisfying the Clock interface, which
// returns the current time in UTC
var UTC = clockFn(func() time.Time { return time.Now().UTC() })

// Local is an object satisfying the Clock interface, which
// returns the current time in the local timezone
var Local = clockFn(time.Now)

// Option is used to pass optional arguments to
// the RotateLogs constructor
type Option interface {
	Name() string
	Value() interface{}
}
