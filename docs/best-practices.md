# 最佳实践

## 命令设计

1. 命令名使用小写英文 + 连字符，如 `my-cmd`
2. 提供中文别名方便用户使用
3. 在 `usage` 字段中写清楚参数格式
4. 无参数时提供帮助提示

## 消息 Hook

1. 避免过于宽泛的关键词匹配（如单个中文字符）
2. 对于高频触发的 Hook，考虑添加冷却（利用 `neobot.redis`）
3. 返回 `nil` / `None` 不回复，让后续 Hook 继续处理

## 错误处理

**Lua:**

```lua
local ok, err = pcall(function()
    -- 可能失败的操作
end)
if not ok then
    neobot.log.error("操作失败: " .. tostring(err))
    return "出错了, 请稍后重试"
end
```

**Python:**

```python
try:
    # 可能失败的操作
except Exception as e:
    sdk.log.error(f"操作失败: {e}")
    return "出错了, 请稍后重试"
```

## 性能

1. 命令 Handler 应快速返回，避免长时间阻塞
2. 耗时操作使用异步（Python: `async/await`；Lua: 无原生支持，避免阻塞）
3. 大量数据存储使用 Redis 而非内存
4. API 调用会阻塞当前事件处理，勿在 Handler 中做大量串行 API 调用

## 状态管理

1. 持久化数据使用 Redis 或 MySQL
2. 避免在 Lua/Python 全局变量中存储需要持久化的状态（热重载会丢失）
3. 插件私有配置放在 `plugin.toml` 的 `[config]` 段

## Python 插件注意事项

1. **子进程隔离**: 每个 Python 插件在独立进程中运行，无法共享内存状态
2. **同步调用**: `sdk.bot.call_api()` 是同步的（底层通过 threading.Event 阻塞等待 JSON-RPC 响应）
3. **不要在 handler 内主动调用 `sdk.bot.send_msg`**: 这会通过 call_api 发送请求给 Go，而 Go 正在等待当前事件处理完成，可能造成死锁。在 handler 中应该 `return` 回复内容而非主动发送
4. **依赖声明**: import 的第三方包必须在 `plugin.toml` 的 `[dependencies].python` 中声明
5. **venv 共享**: 多个插件可共享 venv（配置 `shared_venv_path`），减少磁盘占用
