-- plugins_lua/echo/plugin.lua
local nb = neobot

nb.log.info("echo plugin loaded")

nb.register.command("echo", "user", function(args)
    if #args == 0 then
        return "请输入要复读的内容"
    end
    return table.concat(args, " ")
end)
