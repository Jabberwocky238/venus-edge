# DNS

默认启动创建文件目录`.venus-edge/dns/`

zone文件直接读取`.venus-edge/dns/<zone>.bin`按照capnp模式读

读不到就是不存在

测试也要用类似格式，只不过文件夹改到temp文件夹