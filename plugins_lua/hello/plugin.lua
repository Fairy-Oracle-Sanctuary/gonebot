-- plugins_lua/hello/plugin.lua
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
