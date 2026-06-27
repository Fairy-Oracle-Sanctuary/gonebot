"""plugins_py/webfetch/plugin.py

演示依赖管理:
- plugin.toml [dependencies].python 声明 requests>=2.31
- 启动时 Manager 自动 pip install (有缓存)
- 插件内 import requests 直接可用
"""

from neobot_sdk import command

import requests  # ← 由依赖管理器自动安装


class Plugin:

    @command(name="pyfetch", aliases=["pyget"])
    async def fetch(self, sdk, params):
        args = params.get("args", [])
        if not args:
            return "用法: /pyfetch <url>"
        url = args[0]
        timeout = 10

        try:
            r = requests.get(url, timeout=timeout)
            import re
            m = re.search(r"<title[^>]*>(.*?)</title>", r.text, re.IGNORECASE | re.DOTALL)
            title = m.group(1).strip() if m else "(no title)"
            return f"标题: {title[:200]}"
        except Exception as e:
            return f"请求失败: {e}"
