# Golang实现的分布式WebSocket微服务

## 简介

本系统基于Golang、Redis、RabbitMQ、RPC实现分布式WebSocket微服务，也可以单机部署，单机部署不需要Redis、RabbitMQ和RPC。分布式部署可以支持nginx负责均衡、水平扩容部署，程序之间使用RabbitMQ广播、RPC通信。

基本流程为：用ws协议连接本服务，得到一个clientId，由客户端上报这个clinetId给服务端，服务端拿到这个clientId之后，可以给这个客户端发送信息，绑定这个客户端都分组，给分组发送消息。

目前实现的功能有，给指定客户端发送消息、绑定客户端到分组、给分组里的客户端批量发送消息。适用于长连接的大部分场景，分组可以理解为聊天室，绑定客户端到分组相当于把客户端添加到聊天室，给分组发送信息相当于给聊天室的每个人发送消息。



## 使用场景

在实现业务的时候，我们常常有些需求需要系统主动发送消息给客户端，方案有轮询和长连接，但轮询需要不断的创建销毁http连接，对客户端、对服务器来说都挺消耗资源的，消息推送也不够实时。这里我们选择了WebSocket长连接的方案。

有大量的项目需要服务端主动向客户端推送消息，为了减少重复开发，我们做成了微服务。

使用于服务器需要主动向客户端推送消息、客户端需要实时获取消息的请求。例如聊天、广播消息、多人游戏消息推送、任务执行结果推送等方面。



## 分布式方案

维持大量的长连接对单台服务器的压力也挺大的，这里也就要求该服务需要可以扩容，也就是分布式地扩展。分布式对于可存储的公共资源有一套完整的解决方案，但对于WebSocket来说，操作对象就是每一个连接，它是维持在每一个程序中的。每一个连接不能存储起来共享、不能在不同的程序之间共享。所以我能想到的方案是不同程序之间进行通讯。

那么，怎样知道某个连接在哪个应用呢？答案是通过client id去判断。那么通过client id又是如何知道的呢？有以下几种方案：

1. 一致性hash算法

   一致性hash算法是将整个哈希值空间组织成一个虚拟的圆环，在redis集群中哈希函数的值空间为0-2^32-1（32位无符号整型）。把服务器的IP或主机名作为关键字，通过哈希函数计算出相应的值，对应到这个虚拟的圆环空间。我们再通过哈希函数计算key的值，得到一个在圆环空间的位置，按顺时针方向找到的第一个节点就是存放该key数据的服务器节点。

   在没有节点的增减的时候，可以满足我们的需求，但如果此时一个节点挂掉了或者新增一个机器怎么办？节点挂点之后，会在圆环上删除节点，增加节点则反之。这时候按顺时针方向找的数据就不准确，在某些业务上来说可以接受，但在WebSocket微服务上来说，影响范围内的连接会断掉，如果要求没那么高，客户端再进行重连也可以。

2. hash slot（哈希槽）

   服务器的IP或者主机名作为key，对每个key进行计算CRC16值，然后对16384进行取模，得出一个对应key的hash slot。

   ```
   HASH_SLOT = CRC16(key) mod 16384
   ```

   我们根据节点的数量，给每个节点划分范围，这个范围是0-16384。hash slot的重点就在这个虚拟表，key对应的hash slot是永不变的，增减节点就是维护这张虚拟表。

以上两种方案都可以实现需求，但一致性hash算法的方案会使部分key找到的节点不准确；hash slot的方案需要维护一张虚拟表，在实现起来需要有一个功能去判断服务器是否挂了，然后修改这张虚拟表，新增节点也一样，在实现起来会遇到很多问题。

然后我采取的方案是，每个连接都保存在本应用，然后用redis的key value记录每个连接client id对应的服务器IP和端口。对指定client id进行操作时，去redis找出响应的ip和端口，判断是否为本机，不是本机的话进行RPC通讯告诉相应的程序。长连接的连接数据不可迁移，程序挂掉了相应的连接也就挂了，在该程序上的连接也就断开了，这时重连的话会找到另一个可用的程序。



### 架构图

**单机服务**
![WebSocket单机服务架构图](http://cdn.wugenglong.com/assets/2020/0114OQgeCC.png)

<center>单机服务</certer>

**分布式**

![WebSocket分布式服务架构图](http://cdn.wugenglong.com/assets/2020/0114QsEJ8z.png)
<center>分布式</certer>



### 时序图

**单发消息**

1. 客户端发送连接请求，连接请求通过nginx负载均衡找到一台ws服务器；
2. ws服务器响应连接请求，返回client id，记录到redis，保持长连接；
3. 客户端拿到client id之后，交给业务系统；
4. 业务系统拿到client id之后，通过http发送相关消息，经过nginx负载分配到一台ws服务器；
5. 这台ws服务器拿到clinet id和消息，去redis查询到IP地址和端口；
6. 拿到IP地址和端口，通过PRC协议给指定ws程序发送信息；
7. 该ws程序接收到client id和信息，给指定的连接发送信息；
8. 客户端收到信息。

![WebSocket微服务单发时序图](http://cdn.wugenglong.com/assets/2020/0114jeyDKx.png)
<center>WebSocket微服务单发时序图</certer>

**群发消息**

1. 前3个步骤跟单发的一样；
2. 业务系统拿到client id之后，通过http给指定分组发送消息，经过nginx负载分配到一台ws服务器；
3. 这台ws服务器拿到分组ID和消息，发布给RabbitMQ；
4. 所有订阅到RabbitMQ的服务，会收到新信息推送，找到本机所有该分组的连接；
5. 给所有这些连接发送消息；
6. 客户端收到信息。

![WebSocket微服务群发消息时序图](http://cdn.wugenglong.com/assets/2020/0114PMsbbQ.png)

<center>WebSocket微服务群发消息时序图</certer>



## 使用

**下载本项目：**

```shell
git clone https://github.com/woodylan/go-websocket.git
```

**编译：**

```shell
// 编译适用于本机的版本
go build

// 编译Linux版本
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build

// 编译Windows 64位版本
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build

// 编译MacOS版本
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build
```

**执行：**

编译成功之后会得到一个二进制文件`go-websocket`，执行该二进制文件，文件名后面跟着的是端口号，下面的命令`666`则表示端口号，你可以可以改成其他的。

```shell
./go-websocket 666
```

**连接测试：**

打开支持Websocket的客户端，输入 `ws://127.0.0.1:666/ws` 进行连接，连接成功会返回`clientId`。



## 单机部署

单机部署很简单，不需要配置Redis、RabbitMQ，只需要编译然后运行该二进制文件就可以了，步骤如上。



## 分布式部署

**安装Redis：** 参考网上教程

**安装RabbitMQ：** 参考网上教程



**配置文件：**

配置文件位于项目根目录的`configs/config.ini`，`cluster`为true表示分布式部署，分布式部署[rabbitMQ]、[redis]项必填。

```ini
[common]
# 是否分布式部署
cluster = true

[rabbitMQ]
# AMQP协议URL 格式：amqp://user:password@host:port/vhostname
amqpurl = 
# RabbitMQ交换机名称
exchange = 

[redis]
host = 127.0.0.1
port = 6379
password =
```

**运行项目：**

在不同的机器运行本项目，注意配置号端口号，项目如果在同一机器，则必须用不同的端口。你可以用`supervisor`做进程管理。

**配置Nginx负载均衡：**

```nginx
upstream ws_cluster {
    server 127.0.0.1:666;
    server 127.0.0.1:667;
}

server {
    listen  660;
    server_name ws.example.com;

    access_log /logs/access.log;
    error_log /logs/error.log;

    location / {
        proxy_pass http://ws_cluster; # 代理转发地址
        proxy_http_version 1.1;

        proxy_read_timeout 60s; # 超时设置

        # 启用支持websocket连接
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

至此，项目部署完成。



## 接口

#### 连接接口

**请求地址：**/ws

**协议：** websocket

**响应示例：**

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "clientId": "9fa54bdbbf2778cb"
  }
}
```

#### 发送信息给指定客户端

**请求地址：**/api/send_to_client

**请求方式：** POST

**Content-Type：** application/json; charset=UTF-8

**请求头Body**

| 字段     | 类型   | 是否必须 | 说明     |
| -------- | ------ | -------- | -------- |
| clientId | string | 是       | 客户端ID |
| code | integer | 是       | 自定义的状态码 |
| msg | string | 是       | 自定义的状态消息 |
| data | sring、array、object | 是       | 消息内容 |

**响应示例：**

```json
{
  "code": 0,
  "msg": "success",
  "data": []
}
```

#### 绑定客户端到分组

**请求地址：**/api/bind_to_group

**请求方式：** POST

**Content-Type：** application/json; charset=UTF-8

**请求头Body**

| 字段     | 类型   | 是否必须 | 说明     |
| -------- | ------ | -------- | -------- |
| clientId | string | 是       | 客户端ID |
| groupName | string | 是       | 分组名 |

**响应示例：**

```json
{
  "code": 0,
  "msg": "success",
  "data": []
}
```

#### 发送信息给指定分组

**请求地址：**/api/send_to_group

**请求方式：** POST

**Content-Type：** application/json; charset=UTF-8

**请求头Body**

| 字段     | 类型   | 是否必须 | 说明     |
| -------- | ------ | -------- | -------- |
| groupName | string | 是       | 分组名 |
| code | integer | 是       | 自定义的状态码 |
| msg | string | 是       | 自定义的状态消息 |
| data | sring、array、object | 是       | 消息内容 |

**响应示例：**

```json
{
  "code": 0,
  "msg": "success",
  "data": []
}
```



## 实现的功能

- [x] 分布式
- [ ] 账户授权模式
- [ ] 日志
- [ ] 用户绑定客户端功能
- [ ] 消息持久化
- [ ] 消息记录
- [ ] 限流
- [ ] 黑名单



## 沟通交流
QQ群：1028314856