# ini
使用状态机实现的配置文件解析器

## 特性
1. 简单轻量（API函数只有四个）
2. 使用状态机实现
3. 支持全局Section和自定义Section
4. key-value的value支持多行
5. 支持对不同回车换行符的正确解析

## API说明
此解析器提供了四个API函数供用户使用。

1. 从文件创建一个新的配置
```go
NewConfigFile(filename string)
```

2. 从io.Reader创建一个新的配置
```go
NewConfigReader(r io.Reader)
````
3. 解析配置信息
```go
Parse() error
```
4. 获取配置信息
```go
Get(section, key string) (out string, err error)
```
参数:  
    `section`: 块信息，如果为nil，就是被认为是全局Section  
    `key`:键信息
    `out`:键对应的值    

## 使用
配置文件请参照`test.ini`  

## Bug汇报
如果你发现程序中有任何错误，请发送邮件给我：`fenghai_hhf@163.com`。

## TODO
由于刚开始学习go语言，对一些go语言的理解还很肤浅，因此写这个代码主要是为了
练手。想到的一些TODO，如下：
1. 使用package ini，而不是package main
2. 增加测试代码
3. 提供UnMarshal功能

## 许可证
MIT许可证,详细请参见LICENSE文件
