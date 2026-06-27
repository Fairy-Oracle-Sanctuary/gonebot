-- plugins_lua/greeter/plugin.lua
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
