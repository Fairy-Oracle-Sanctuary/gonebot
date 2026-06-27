package initfiles

// ---- Lua 示例插件文件内容 ----

const luaEchoToml = `name = "echo"
version = "1.0.0"
author = "NeoBot Team"
description = "复读机插件"
usage = "/echo <消息>"
permission = "user"
runtime = "lua"
`

const luaEchoLua = `-- plugins_lua/echo/plugin.lua
local nb = neobot

nb.log.info("echo plugin loaded")

nb.register.command("echo", "user", function(args)
    if #args == 0 then
        return "请输入要复读的内容"
    end
    return table.concat(args, " ")
end)
`

const luaHelloToml = `name = "hello"
version = "1.0.0"
author = "NeoBot Team"
description = "简单示例插件"
usage = "/hello <名字>"
permission = "user"
runtime = "lua"

[config]
greeting = "你好"
`

const luaHelloLua = `-- plugins_lua/hello/plugin.lua
-- 最小示例: 注册 /hello 与 /ping 命令

local nb = neobot  -- SDK 以全局变量注入

nb.log.info("hello plugin loaded, greeting=" .. nb.config.get("greeting", "你好"))

-- 注册命令: /hello <名字>
nb.register.command("hello", "user", function(args)
    local name = args[1] or "世界"
    return nb.config.get("greeting", "你好") .. ", " .. name .. "!"
end, { aliases = { "hi", "你好" } })

-- 注册命令: /ping
nb.register.command("ping", "user", function(args)
    return "pong @ " .. nb.util.date("%Y-%m-%d %H:%M:%S")
end)

-- 全局消息 hook: 包含 "时间" 时返回当前时间
nb.register.on_message(function(text)
    if string.find(text, "时间") or string.find(text, "几点") then
        return "现在是 " .. nb.util.date("%H:%M:%S")
    end
end)

-- 通知 hook: 群成员增加
nb.register.on_notice("group_increase", function(notice_type)
    nb.log.info("group_increase: " .. notice_type)
    return "欢迎新成员！"
end)
`

const luaGreeterToml = `name = "greeter"
version = "1.0.0"
author = "NeoBot Team"
description = "Lua 插件示例: 使用本地 lib 工具"
usage = "/greeter <名字>"
permission = "user"
runtime = "lua"

[dependencies]
lua = ["string-utils"]
local = ["./lib"]

[config]
greeting = "你好"
`

const luaGreeterLua = `-- plugins_lua/greeter/plugin.lua
-- 演示 [dependencies]:
--   - lua = ["string-utils"]: 校验 lib/string-utils.lua 存在
--   - local = ["./lib"]: 设置 package.path

local nb = neobot

-- 通过 [dependencies].local 自动注入的 package.path 加载本地库
nb_package_paths = nb_package_paths or ""
package.path = nb_package_paths .. ";./?.lua;./?/init.lua;" .. (package.path or "")
local str = require("string-utils")

nb.log.info("greeter plugin loaded, package.path head=" .. package.path:sub(1, 50))

nb.register.command("greeter", "user", function(args)
    local name = args[1] or "世界"
    return str.greet(name, nb.config.get("greeting", "你好"))
end, { aliases = { "hi-cn" } })
`

const luaGreeterLib = `-- plugins_lua/greeter/lib/string-utils.lua
-- 示例本地 Lua 库, 由 [dependencies].lua 校验存在性.

local M = {}

--- 大写首字母.
function M.capitalize(s)
    if not s or s == "" then return s end
    return s:sub(1, 1):upper() .. s:sub(2)
end

--- 拼接 greeting.
function M.greet(name, prefix)
    prefix = prefix or "你好"
    return prefix .. ", " .. M.capitalize(name) .. "!"
end

return M
`

const luaBroadcastToml = `name = "broadcast"
version = "1.0.0"
author = "NeoBot Team"
description = "管理员广播插件，先输指令再发消息，支持任意消息段（文本/图片/At等）"
usage = """/broadcast         进入广播模式
然后发送任意消息   广播到所有群聊 (60秒超时)"""
permission = "admin"
runtime = "lua"
`

const luaBroadcastLua = `-- plugins_lua/broadcast/plugin.lua
-- 管理员广播插件: 两步式广播.
--   1. 发送 /broadcast 进入广播模式
--   2. 发送任意消息 (支持文本/图片/At/表情等), 自动广播到机器人所在的所有群聊
-- 仅限管理员使用.

local nb = neobot

nb.log.info("broadcast plugin loaded")

-- 广播会话: { [user_id] = timestamp_ms }
local sessions = {}

-- /broadcast 进入广播模式
nb.register.command("broadcast", "admin", function(args)
    local user_id = nb.event.user_id()
    sessions[user_id] = nb.util.now()
    return "已进入广播模式，请在 60 秒内发送您要广播的消息内容。\n支持文本、图片、表情、@ 等任意消息段。"
end)

-- 捕获广播内容 (on_message hook)
nb.register.on_message(function(text)
    local user_id = nb.event.user_id()
    local ts = sessions[user_id]
    if not ts then
        return nil  -- 不在广播会话中
    end

    -- 检查超时 (60秒)
    if nb.util.now() - ts > 60000 then
        sessions[user_id] = nil
        return nil
    end

    -- 清理会话
    sessions[user_id] = nil

    local msg_type = nb.event.message_type()
    local group_id = nb.event.group_id()
    local segs = nb.event.segments()

    nb.log.info("broadcast start", "user_id", user_id, "segs", #segs)

    -- 获取群列表
    local groups, err = nb.bot.get_group_list()
    if not groups then
        if msg_type == "private" then
            nb.bot.send_private_msg(user_id, "获取群列表失败: " .. tostring(err))
        end
        return nil
    end
    if type(groups) ~= "table" or #groups == 0 then
        if msg_type == "private" then
            nb.bot.send_private_msg(user_id, "当前没有加入任何群聊")
        end
        return nil
    end

    -- 构造广播消息: 如果有消息段则发送完整段, 否则发送纯文本
    local message
    if #segs > 0 then
        message = segs
    else
        message = text
    end

    -- 广播到所有群
    local success, failed = 0, 0
    for _, g in ipairs(groups) do
        local gid = g.group_id
        local ok, msg_err = pcall(function()
            return nb.bot.send_group_msg(gid, message)
        end)
        if ok then
            success = success + 1
        else
            failed = failed + 1
            nb.log.warn("broadcast failed", "group_id", gid, "err", tostring(msg_err))
        end
    end

    local total = success + failed
    local result = "广播完成！共 " .. total .. " 个群，成功 " .. success .. " 个"
    if failed > 0 then
        result = result .. "，失败 " .. failed .. " 个"
    end

    -- 手动回复完成消息 (不用 return, 避免触发命令双重回复)
    if msg_type == "private" then
        nb.bot.send_private_msg(user_id, result)
    elseif msg_type == "group" and group_id ~= 0 then
        nb.bot.send_group_msg(group_id, result)
    end

    nb.log.info("broadcast done", "total", total, "success", success, "failed", failed)

    return nil  -- 不触发 router 自动回复
end)
`
