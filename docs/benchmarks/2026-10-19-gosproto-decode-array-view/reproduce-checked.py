"""修复归档测试补丁静默跳过；冻结计时驱动原件保留，checked入口本轮只做prepare验证。"""
from pathlib import Path
import hashlib

original = Path(__file__).with_name('reproduce.py')
source = original.read_text()
assert hashlib.sha256(original.read_bytes()).hexdigest() == 'f1e8939dd251d4693db372d4791adbbd8e4b9a8a49e0d83b566077f24fbdd9ef'
old = "run(['git','apply',str(here/'contract-test.patch')], directory/side)"
new = "run(['patch','--batch','-p1','-i',str(here/'contract-test.patch')], directory/side)\n        assert pairs.digest(directory/side/'limits_test.go') == '9612667b2dd51f286ee972d8303d1609a0c6bb274050f0056abe392447f29809'"
assert source.count(old) == 1
source = source.replace(old, new)
old_bound = "here/'reproduce.py', tool"
assert source.count(old_bound) == 1
source = source.replace(old_bound, "here/'reproduce.py', here/'reproduce-checked.py', tool")
exec(compile(source, str(original), 'exec'))
