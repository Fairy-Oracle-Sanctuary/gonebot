-- plugins_lua/greeter/lib/string-utils.lua
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
