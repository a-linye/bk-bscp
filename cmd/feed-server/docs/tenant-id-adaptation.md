# Feed-Server 租户 ID 适配逻辑

## 概述

feed-server 通过 gRPC 拦截器统一解析租户 ID（TenantID），Unary 和 Stream 请求均在拦截器中完成租户解析，
所有 handler 无需重复调用 `EnsureTenantID`。

---

## 核心组件

| 组件 | 文件 | 说明 |
|------|------|------|
| 程序入口 | `cmd/feed-server/feed_server.go` | `main()` 函数 |
| 应用启动 | `cmd/feed-server/app/app.go` | gRPC Server 创建、拦截器链注册、网络监听 |
| Service 实现 | `cmd/feed-server/service/service.go` | `Service` 结构体，实现 `UpstreamServer` 接口 |
| 拦截器 | `cmd/feed-server/service/interceptor.go` | 所有 Unary / Stream 拦截器 |
| RPC handler | `cmd/feed-server/service/rpc_sidecar.go` | 各 gRPC 方法的业务实现 |
| Proto 定义 | `pkg/protocol/feed-server/feed_server.proto` | `Upstream` 服务定义（13 Unary + 2 Stream） |
| 生成的 gRPC 代码 | `pkg/protocol/feed-server/feed_server_grpc.pb.go` | `RegisterUpstreamServer`、`Upstream_ServiceDesc`、各方法 handler |
| Kit 结构体 | `pkg/kit/kit.go` | 请求上下文载体，包含 TenantID 字段 |
| 常量定义 | `pkg/criteria/constant/key.go` | `BkTenantID = "X-Bk-Tenant-Id"`，`DefaultTenantID = "default"` |
| 租户解析 | `cmd/feed-server/bll/lcache/app.go` | `EnsureTenantID`：本地缓存 -> cache-service RPC -> 默认值 |
| 业务逻辑层 | `cmd/feed-server/bll/bll.go` | `BLL` 结构体，封装缓存、Auth、Release 等能力 |
| Sidecar 上下文解析 | `pkg/sf-share/meta.go` | `ParseFeedIncomingContext`：从 metadata 读取 TenantID |

---

## 启动流程

```
main()                                              // cmd/feed-server/feed_server.go
  │
  ├─ cc.InitService(cc.FeedServerName)              // 初始化服务名
  ├─ options.InitOptions()                          // 解析命令行参数
  │
  └─ app.Run(opts)                                  // cmd/feed-server/app/app.go
       │
       ├─ fs.prepare(opt)                           // ① 准备阶段
       │    ├─ cc.LoadSettings()                    //    读配置文件
       │    ├─ metrics.InitMetrics()                //    初始化 Prometheus 指标
       │    ├─ serviced.NewServiceD()               //    初始化 etcd 服务发现
       │    └─ service.NewService(sd, name)          //    ★ 创建 Service 实例
       │         ├─ auth.NewAuthorizer()            //      初始化鉴权器
       │         ├─ bll.New(sd, authorizer, name)   //      初始化业务逻辑层（缓存、Auth 等）
       │         ├─ repository.NewProvider()        //      初始化文件存储（对象存储）
       │         ├─ ratelimiter.New()               //      初始化限流器
       │         └─ bkcmdb.New()                    //      初始化 CMDB 客户端
       │
       ├─ fs.listenAndServe()                       // ② 启动 gRPC 服务
       │    ├─ 构建拦截器链（Unary 8 层 + Stream 6 层）
       │    ├─ grpc.NewServer(opts...)              //    创建 gRPC Server
       │    ├─ pbfs.RegisterUpstreamServer(         //    ★ 注册 Service 为 Upstream 实现
       │    │      serve, fs.service)
       │    ├─ reflection.Register(serve)           //    注册 gRPC 反射（调试用）
       │    └─ serve.Serve(dualStackListener)       //    监听端口、开始接收请求
       │
       ├─ fs.service.ListenAndServeRest()           // ③ 启动 HTTP 服务（健康检查等）
       ├─ fs.service.ListenAndGwServerRest()        // ④ 启动 gRPC-Gateway（HTTP→gRPC 转换）
       ├─ fs.register()                             // ⑤ 注册到 etcd（服务发现）
       └─ shutdown.WaitShutdown(20)                 // ⑥ 阻塞等待退出信号（20 秒优雅关闭）
```

**核心绑定发生在这一行：**

```go
// app.go L190
pbfs.RegisterUpstreamServer(serve, fs.service)
```

它把 `*service.Service` 注册为 proto 中定义的 `Upstream` 服务的实现。

---

## 三种请求入口

feed-server 监听 **3 个端口**，对应 3 条独立的请求入口：

```
                          ┌──────────────────┐
                          │   feed-server     │
                          └──────────────────┘
                                  │
                ┌─────────────────┼─────────────────┐
                ▼                 ▼                  ▼
         gRPC (RpcPort)    HTTP (HttpPort)    gRPC-GW (GwHttpPort)
         ┌──────────┐     ┌──────────┐       ┌──────────────┐
         │ 11 个     │     │ 健康检查  │       │ HTTP → gRPC  │
         │ Unary +   │     │ /-/healthy│       │ 转换代理     │
         │ 2 个      │     │ /-/ready  │       │ /api/v1/feed │
         │ Stream    │     │ /healthz  │       │ /biz/xxx/... │
         └──────────┘     └──────────┘       └──────────────┘
              │                                     │
              │       sidecar 直连                   │  SDK/浏览器通过 HTTP
              │                                     │
              └──── gRPC 拦截器链 ────┘              └── chi 中间件链 ──┘
```

| 入口 | 端口配置 | 协议 | 用途 |
|------|---------|------|------|
| gRPC | `Network.RpcPort` | HTTP/2 + protobuf | sidecar 直连，主路径 |
| HTTP | `Network.HttpPort` | HTTP/1.1 | 健康检查、运维工具 |
| gRPC-Gateway | `Network.GwHttpPort` | HTTP/1.1 → gRPC | SDK 通过 RESTful API 访问（如 `PullKvMeta`、`GetKvValue`） |

gRPC-Gateway 的请求最终也会转成 gRPC 调用，经过同样的拦截器链到达同一个 handler。

---

## 服务方法注册：ServiceDesc

`RegisterUpstreamServer` 内部调用 `s.RegisterService(&Upstream_ServiceDesc, srv)`。
`Upstream_ServiceDesc`（位于 `feed_server_grpc.pb.go`）定义了**方法名 → handler 函数**的路由表：

```go
var Upstream_ServiceDesc = grpc.ServiceDesc{
    ServiceName: "pbfs.Upstream",
    HandlerType: (*UpstreamServer)(nil),

    // 11 个 Unary 方法
    Methods: []grpc.MethodDesc{
        {MethodName: "Handshake",           Handler: _Upstream_Handshake_Handler},
        {MethodName: "Messaging",           Handler: _Upstream_Messaging_Handler},
        {MethodName: "PullAppFileMeta",     Handler: _Upstream_PullAppFileMeta_Handler},
        {MethodName: "GetDownloadURL",      Handler: _Upstream_GetDownloadURL_Handler},
        {MethodName: "PullKvMeta",          Handler: _Upstream_PullKvMeta_Handler},
        {MethodName: "GetKvValue",          Handler: _Upstream_GetKvValue_Handler},
        {MethodName: "ListApps",            Handler: _Upstream_ListApps_Handler},
        {MethodName: "AsyncDownload",       Handler: _Upstream_AsyncDownload_Handler},
        {MethodName: "AsyncDownloadStatus", Handler: _Upstream_AsyncDownloadStatus_Handler},
        {MethodName: "GetSingleKvValue",    Handler: _Upstream_GetSingleKvValue_Handler},
        {MethodName: "GetSingleKvMeta",     Handler: _Upstream_GetSingleKvMeta_Handler},
    },

    // 2 个 Stream 方法
    Streams: []grpc.StreamDesc{
        {StreamName: "Watch",                Handler: _Upstream_Watch_Handler,              ServerStreams: true},
        {StreamName: "GetSingleFileContent", Handler: _Upstream_GetSingleFileContent_Handler, ServerStreams: true},
    },
}
```

gRPC 框架用这个 `ServiceDesc`，建立 **`/pbfs.Upstream/方法名` → handler 函数** 的内部路由表。
请求到达时，根据 HTTP/2 帧的 `:path` 匹配到对应的 handler。

---

## 拦截器链顺序

### Unary 链

```
1. realip.UnaryServerInterceptorOpts()              -- 提取客户端真实 IP
2. service.LogUnaryServerInterceptor()               -- 请求日志
3. grpcMetrics.UnaryServerInterceptor()              -- Prometheus 指标
4. ratelimit.UnaryServerInterceptor(ipLimiter)       -- IP 限流
5. service.FeedEnsureTenantInterceptor           ★   -- 租户 ID 解析 & 注入 metadata
6. service.FeedUnaryAuthInterceptor                  -- 鉴权（Bearer Token）
7. service.FeedUnaryUpdateLastConsumedTimeInterceptor -- 更新拉取时间
8. grpc_recovery.UnaryServerInterceptor()            -- panic 恢复
```

### Stream 链

```
1. realip.StreamServerInterceptorOpts()              -- 提取客户端真实 IP
2. grpcMetrics.StreamServerInterceptor()             -- Prometheus 指标
3. ratelimit.StreamServerInterceptor(ipLimiter)      -- IP 限流
4. service.FeedStreamEnsureTenantInterceptor     ★   -- 租户 ID 解析 & 注入 metadata
5. service.FeedStreamAuthInterceptor                 -- 鉴权
6. grpc_recovery.StreamServerInterceptor()           -- panic 恢复
```

拦截器以**洋葱模型**执行：`1 → 2 → ... → N → handler → N → ... → 2 → 1`

---

## 请求链路详解

### Unary 请求（以 `GetKvValue` 为例）

```
sidecar 发起 gRPC 调用: /pbfs.Upstream/GetKvValue
    │
    ▼
TCP 连接到达 dualStackListener
    │
    ▼
grpc.Server.Serve() 接收连接
    │ 读取 HTTP/2 帧，解析出 :path = /pbfs.Upstream/GetKvValue
    │ 查 ServiceDesc 路由表 → 匹配 Methods 中的 "GetKvValue"
    │ 发现是 Unary → 走 Unary 处理路径
    │
    ▼
gRPC 框架构造 interceptor 链（ChainUnaryInterceptor 注册的顺序）
    │
    │  ┌─────────────────────────────────────────────────────────────┐
    │  │  1. realip                   提取真实 IP                     │
    │  │  2. LogUnaryServerInterceptor 记录请求日志                    │
    │  │  3. grpcMetrics              Prometheus 指标                 │
    │  │  4. ratelimit                IP 限流                        │
    │  │  5. FeedEnsureTenantInterceptor ★ 解析 TenantID → 注入 metadata │
    │  │  6. FeedUnaryAuthInterceptor    Bearer Token 鉴权           │
    │  │  7. FeedUnaryUpdateLastConsumedTimeInterceptor 更新拉取时间   │
    │  │  8. grpc_recovery             panic 恢复                    │
    │  └─────────────────────────────────────────────────────────────┘
    │
    ▼
_Upstream_GetKvValue_Handler (protoc 生成的代码, feed_server_grpc.pb.go)
    │
    │  这个生成的 handler 做两件事:
    │  1. dec(in): 反序列化请求体 → *GetKvValueReq
    │  2. 调用最终 handler:
    │       srv.(UpstreamServer).GetKvValue(ctx, req.(*GetKvValueReq))
    │
    ▼
service.Service.GetKvValue(ctx, req)     ← 业务代码 (rpc_sidecar.go)
    │
    │  kt := kit.FromGrpcContext(ctx)    ← 从 metadata 读到 TenantID（拦截器已注入）
    │  credential := getCredential(ctx)  ← 从 context 读到鉴权信息（拦截器已设置）
    │  s.bll.AppCache().GetAppID(...)
    │  s.bll.Release().GetRelease(...)
    │  ...
    │
    ▼
返回 *GetKvValueResp → gRPC 框架序列化 → 发回 sidecar
```

#### protoc 生成的 handler 是 req 和拦截器的桥梁

```go
// feed_server_grpc.pb.go — 以 GetKvValue 为例
func _Upstream_GetKvValue_Handler(srv interface{}, ctx context.Context,
    dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {

    in := new(GetKvValueReq)
    if err := dec(in); err != nil {     // ① 反序列化请求体
        return nil, err
    }
    if interceptor == nil {
        return srv.(UpstreamServer).GetKvValue(ctx, in)
    }
    info := &grpc.UnaryServerInfo{
        Server:     srv,                // ② srv 就是 *service.Service
        FullMethod: "/pbfs.Upstream/GetKvValue",
    }
    handler := func(ctx context.Context, req interface{}) (interface{}, error) {
        return srv.(UpstreamServer).GetKvValue(ctx, req.(*GetKvValueReq))  // ④ 最终调用业务方法
    }
    return interceptor(ctx, in, info, handler)  // ③ 进入拦截器链
}
```

`interceptor` 参数是 gRPC 框架用 `ChainUnaryInterceptor` 把 8 个拦截器串起来的**合并拦截器**。
每个拦截器调用 `handler(ctx, req)` 时，实际调用的是链中的下一个拦截器，直到最后一个才调用 ④。

### Stream 请求（以 `Watch` 为例）

```
sidecar 发起 gRPC 调用: /pbfs.Upstream/Watch
    │
    ▼
grpc.Server 解析 :path → 匹配 Streams 中的 "Watch"
    │ 发现是 Stream → 走 Stream 处理路径
    │
    ▼
gRPC 框架构造 stream interceptor 链
    │
    │  ┌─────────────────────────────────────────────────────────────┐
    │  │  1. realip                                                  │
    │  │  2. grpcMetrics                                             │
    │  │  3. ratelimit                                               │
    │  │  4. FeedStreamEnsureTenantInterceptor ★ 从 sidecar-meta     │
    │  │     metadata 解析 bizID → 解析 TenantID → 注入 metadata      │
    │  │  5. FeedStreamAuthInterceptor                               │
    │  │  6. grpc_recovery                                           │
    │  └─────────────────────────────────────────────────────────────┘
    │
    │  注意：Stream 拦截器此时还没有 req 对象！
    │  但 TenantID 来自 gRPC metadata 的 sidecar-meta header，此时已可用
    │
    ▼
_Upstream_Watch_Handler (protoc 生成的代码, feed_server_grpc.pb.go)
    │
    │  m := new(SideWatchMeta)
    │  stream.RecvMsg(m)               ← ★ 这里才第一次读到请求体
    │  srv.(UpstreamServer).Watch(m, &upstreamWatchServer{stream})
    │
    ▼
service.Service.Watch(swm, fws)      ← 业务代码 (rpc_sidecar.go)
    │
    │  im := sfs.ParseFeedIncomingContext(fws.Context())
    │       ← TenantID 已在 metadata 中（拦截器通过 wrappedStream 注入）
    │
    │  // 长连接循环: fws.Send(msg) 持续推送变更
    │
    ▼
连接保持，直到客户端断开或服务端关闭
```

#### protoc 生成的 Stream handler

```go
// feed_server_grpc.pb.go
func _Upstream_Watch_Handler(srv interface{}, stream grpc.ServerStream) error {
    m := new(SideWatchMeta)
    if err := stream.RecvMsg(m); err != nil {   // ① 此时才读请求体
        return err
    }
    return srv.(UpstreamServer).Watch(m, &upstreamWatchServer{stream})  // ② 调用业务方法
}
```

注意与 Unary handler 的区别：这里没有 `interceptor` 参数。
Stream 拦截器链是 gRPC 框架在**调用这个 handler 之前**就已经执行完毕的，
通过 `wrappedStream` 传递修改后的 context。

### Unary 与 Stream 的关键区别

|  | Unary | Stream |
|---|---|---|
| 拦截器签名 | `func(ctx, req, info, handler)` | `func(srv, stream, info, handler)` |
| 请求体何时可用 | 拦截器调用前已反序列化 | handler 内部 `stream.RecvMsg()` 时 |
| ctx 传递方式 | 直接传 `ctx` 参数 | 通过 `wrappedStream` 覆盖 `Context()` |
| 生成代码中拦截器位置 | 在 handler 内部调用 `interceptor(ctx, in, info, handler)` | 在 handler 外部由框架串联 |
| 租户 ID 的 bizID 来源 | `extractBizIDAndApp(req)` 从请求体提取 | 从 `sidecar-meta` metadata header 提取 |

---

## 租户 ID 数据流

### Unary RPC 请求流程

```
sidecar 请求（不携带 x-bk-tenant-id）
    │
    ▼
FeedEnsureTenantInterceptor
    ├─ extractBizIDAndApp(req) 从请求体提取 bizID
    ├─ kit.FromGrpcContext(ctx) 构建 Kit（此时 TenantID 为空）
    ├─ EnsureTenantID(kt, bizID) 解析租户
    │       ├─ kt.TenantID 非空 → 直接返回
    │       ├─ 本地 gcache 命中 → 设置 kt.TenantID
    │       └─ 本地未命中 → 调用 cache-service.GetTenantIDByBiz(bizID)
    │               └─ 返回空 → 使用 DefaultTenantID ("default")
    │
    ├─ ★ 将 kt.TenantID 写入 gRPC incoming metadata
    │       md.Set("x-bk-tenant-id", kt.TenantID)
    │       ctx = metadata.NewIncomingContext(ctx, md)
    │
    ▼
FeedUnaryAuthInterceptor → ... → Handler
    │
    ├─ kit.FromGrpcContext(ctx)         ← 从 metadata 读到 TenantID ✓
    └─ ParseFeedIncomingContext(ctx)    ← 从 metadata 读到 TenantID ✓
```

### Stream RPC 请求流程（Watch / GetSingleFileContent）

```
sidecar 请求（不携带 x-bk-tenant-id，但 metadata 中有 sidecar-meta header）
    │
    ▼
FeedStreamEnsureTenantInterceptor
    ├─ 从 gRPC incoming metadata 读取 sidecar-meta header
    ├─ 反序列化 SidecarMetaHeader，提取 sm.BizID
    ├─ kit.FromGrpcContext(ctx) 构建 Kit（此时 TenantID 为空）
    ├─ EnsureTenantID(kt, sm.BizID) 解析租户
    │       └─ 与 Unary 拦截器中相同的解析逻辑
    │
    ├─ ★ 将 kt.TenantID 写入 gRPC incoming metadata
    │       md.Set("x-bk-tenant-id", kt.TenantID)
    │       ctx = metadata.NewIncomingContext(ctx, md)
    │
    ├─ 通过 wrappedStream 传递修改后的 ctx
    │
    ▼
FeedStreamAuthInterceptor → Handler
    │
    └─ ParseFeedIncomingContext(stream.Context())  ← 从 metadata 读到 TenantID ✓
```

**Stream 拦截器可以统一处理的原因**：bizID 位于 gRPC metadata 中的 `sidecar-meta` header（JSON 格式），
metadata 在 Stream 建立时就已可用，不需要等待 `stream.Recv()` 获取请求体。

---

## 关键实现细节

### 1. FeedEnsureTenantInterceptor（Unary）注入 metadata

```go
// cmd/feed-server/service/interceptor.go

func FeedEnsureTenantInterceptor(
    ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
) (interface{}, error) {

    svc, ok := info.Server.(*Service)
    if !ok {
        return handler(ctx, req)
    }

    bizID, _ := extractBizIDAndApp(req, info.FullMethod)
    if bizID != 0 {
        ctx = context.WithValue(ctx, constant.BizIDKey, bizID)
        kt := kit.FromGrpcContext(ctx)
        if err := svc.bll.AppCache().EnsureTenantID(kt, bizID); err != nil {
            logs.Errorf("ensure tenant id failed, biz: %d, method: %s, err: %v",
                bizID, info.FullMethod, err)
        }

        // ★ 核心：将 TenantID 写回 incoming metadata
        if kt.TenantID != "" {
            md, _ := metadata.FromIncomingContext(ctx)
            md = md.Copy()
            md.Set(strings.ToLower(constant.BkTenantID), kt.TenantID)
            ctx = metadata.NewIncomingContext(ctx, md)
        }
    }

    return handler(ctx, req)
}
```

### 2. FeedStreamEnsureTenantInterceptor（Stream）注入 metadata

```go
// cmd/feed-server/service/interceptor.go

func FeedStreamEnsureTenantInterceptor(
    srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler,
) error {

    svc, ok := srv.(*Service)
    if !ok {
        return handler(srv, ss)
    }

    ctx := ss.Context()
    md, ok := metadata.FromIncomingContext(ctx)
    if !ok {
        return handler(srv, ss)
    }

    // bizID 在 sidecar-meta header 里，拦截器阶段就能拿到
    var metaHeader string
    if sm := md.Get(constant.SidecarMetaKey); len(sm) != 0 {
        metaHeader = sm[0]
    }
    if metaHeader == "" {
        return handler(srv, ss)
    }

    sm := new(sfs.SidecarMetaHeader)
    if err := jsoni.UnmarshalFromString(metaHeader, sm); err != nil {
        return handler(srv, ss)
    }

    if sm.BizID != 0 {
        ctx = context.WithValue(ctx, constant.BizIDKey, sm.BizID)
        kt := kit.FromGrpcContext(ctx)
        if err := svc.bll.AppCache().EnsureTenantID(kt, sm.BizID); err != nil {
            logs.Errorf("stream ensure tenant id failed, biz: %d, method: %s, err: %v",
                sm.BizID, info.FullMethod, err)
        }

        // 将 TenantID 写入 incoming metadata，下游 ParseFeedIncomingContext 可直接读取
        if kt.TenantID != "" {
            md = md.Copy()
            md.Set(strings.ToLower(constant.BkTenantID), kt.TenantID)
            ctx = metadata.NewIncomingContext(ctx, md)
        }
    }

    return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
}
```

**与 Unary 拦截器的区别**：
- bizID 来源不同：从 `sidecar-meta` metadata header 中反序列化 `SidecarMetaHeader.BizID`，而非从请求体提取
- ctx 传递方式不同：通过 `wrappedStream` 封装传递修改后的 context，而非直接传 ctx 参数

### 3. ParseFeedIncomingContext 读取 TenantID

```go
// pkg/sf-share/meta.go

func ParseFeedIncomingContext(ctx context.Context) (*IncomingMeta, error) {
    md, ok := metadata.FromIncomingContext(ctx)
    // ... 解析 rid、sidecar metadata ...

    // 从 metadata 提取 tenantID（由拦截器注入）
    var tenantID string
    if tv := md.Get(strings.ToLower(constant.BkTenantID)); len(tv) != 0 {
        tenantID = tv[0]
    }

    return &IncomingMeta{
        Kit: &kit.Kit{
            Ctx:      ctx,
            Rid:      rid,
            TenantID: tenantID,
        },
        Meta: sm,
    }, nil
}
```

### 4. EnsureTenantID 缓存策略

```go
// cmd/feed-server/bll/lcache/app.go

func (ap *App) EnsureTenantID(kt *kit.Kit, bizID uint32) error {
    // 1. 已设置则跳过
    if kt.TenantID != "" {
        return nil
    }

    key := fmt.Sprintf("%d-%s", bizID, "tenant-id")

    // 2. 本地 gcache（LRU + TTL）
    val, err := ap.idClient.GetIFPresent(key)
    if err == nil {
        if tenantID, ok := val.(string); ok && tenantID != "" {
            kt.TenantID = tenantID
            return nil
        }
    }

    // 3. cache-service RPC
    resp, err := ap.cs.CS().GetTenantIDByBiz(kt.RpcCtx(), &pbcs.GetTenantIDByBizReq{BizId: bizID})
    if err != nil {
        return err
    }

    tenantID := resp.TenantId
    if tenantID == "" {
        tenantID = constant.DefaultTenantID  // "default"
    }

    kt.TenantID = tenantID
    _ = ap.idClient.Set(key, resp.TenantId)
    return nil
}
```

缓存层级：**本地 gcache → cache-service → 默认值 "default"**

### 5. kit.FromGrpcContext 读取 TenantID

```go
// pkg/kit/kit.go

func FromGrpcContext(ctx context.Context) *Kit {
    kit := &Kit{Ctx: ctx}

    md, ok := metadata.FromIncomingContext(ctx)
    // ...

    // 读取 TenantID
    tenantID := md[strings.ToLower(constant.BkTenantID)]  // "x-bk-tenant-id"
    if len(tenantID) != 0 {
        kit.TenantID = tenantID[0]
    }

    return kit
}
```

---

## TenantID 在 ctx 中的传递过程（逐步跟踪）

以一个 `GetKvValue` 请求为例，假设 `bizID = 100`，对应租户为 `"tenant_abc"`。

### 第 0 步：请求到达时 ctx 里有什么

sidecar 发来的请求 metadata 里**没有** `x-bk-tenant-id`（sidecar 不知道租户的概念）：

```
ctx 中的 incoming metadata = {
    "side-rid":      ["bscp-xxxx"],
    "sidecar-meta":  ["{\"bid\":100,\"fpt\":\"abc\"}"],
    // 没有 x-bk-tenant-id
}
```

### 第 1 步：拦截器中 EnsureTenantID 解析租户

```go
bizID, _ := extractBizIDAndApp(req, info.FullMethod)
// bizID = 100（从 req.BizId 提取）

kt := kit.FromGrpcContext(ctx)
// kt.TenantID = ""  ← metadata 里没有 x-bk-tenant-id，所以是空的

svc.bll.AppCache().EnsureTenantID(kt, 100)
// 查缓存/RPC 得知 bizID=100 → tenantID="tenant_abc"
// kt.TenantID = "tenant_abc"  ← 写进了这个 kt 对象
```

**问题**：这个 `kt` 是拦截器里的局部变量，handler 里会自己创建新的 Kit，读不到这个值。

### 第 2 步：把 TenantID 搬到 ctx 的 metadata 里（关键）

```go
md, _ := metadata.FromIncomingContext(ctx)
// md = {"side-rid": [...], "sidecar-meta": [...]}  ← 还是没有 x-bk-tenant-id

md = md.Copy()
md.Set("x-bk-tenant-id", "tenant_abc")
// md = {"side-rid": [...], "sidecar-meta": [...], "x-bk-tenant-id": ["tenant_abc"]}
// ↑ 只是修改了一个 map，还没跟 ctx 产生关系

ctx = metadata.NewIncomingContext(ctx, md)   // ★ 生成新 ctx，里面带着修改后的 md
```

此时 ctx 中的 incoming metadata 变成了：

```
ctx 中的 incoming metadata = {
    "side-rid":          ["bscp-xxxx"],
    "sidecar-meta":      ["{\"bid\":100,\"fpt\":\"abc\"}"],
    "x-bk-tenant-id":   ["tenant_abc"],     ← ★ 新增的
}
```

然后 `return handler(ctx, req)` —— 把这个**新的 ctx** 往下传。

### 第 3 步：handler 里创建新 Kit，从 ctx 读到 TenantID

新接口（如 `GetKvValue`）：

```go
func (s *Service) GetKvValue(ctx context.Context, req *pbfs.GetKvValueReq) (...) {
    kt := kit.FromGrpcContext(ctx)
    // 内部: md["x-bk-tenant-id"] → ["tenant_abc"]  ← 读到了！
    // kt.TenantID = "tenant_abc" ✓
}
```

老接口（如 `Messaging`）：

```go
func (s *Service) Messaging(ctx context.Context, msg *pbfs.MessagingMeta) (...) {
    im, _ := sfs.ParseFeedIncomingContext(ctx)
    // 内部: md.Get("x-bk-tenant-id") → ["tenant_abc"]  ← 读到了！
    // im.Kit.TenantID = "tenant_abc" ✓
}
```

### 总结：三步完成传递

```
① EnsureTenantID：     bizID=100 → 查缓存/RPC → tenantID="tenant_abc" → 存到临时变量 kt 里
② NewIncomingContext：  把 "tenant_abc" 从 kt 搬到 ctx 的 metadata 里
③ handler 里构建 Kit：  从 ctx 的 metadata 里读出 "tenant_abc"
```

**核心是第 ② 步**——用 metadata 做中转。拦截器和 handler 各自创建独立的 Kit 对象，
它们之间唯一共享的东西就是 `ctx`，所以必须把 TenantID 塞进 `ctx` 的 metadata 里才能传过去。

---

## 新老接口获取租户 ID 的区别

feed-server 的接口分为"老接口"和"新接口"两类，它们获取租户 ID 的**最终来源相同**（都是从 ctx 的 metadata 读），
但 Kit 的构建路径和鉴权方式不同。

### 分类依据

老接口在 `disabledMethod` 中注册，会跳过 `FeedUnaryAuthInterceptor`（Bearer Token 鉴权拦截器）：

```go
var disabledMethod = map[string]struct{}{
    "/pbfs.Upstream/Handshake":            {},
    "/pbfs.Upstream/Messaging":            {},
    "/pbfs.Upstream/Watch":                {},
    "/pbfs.Upstream/PullAppFileMeta":      {},
    "/pbfs.Upstream/GetDownloadURL":       {},
    "/pbfs.Upstream/GetSingleFileContent": {},
}
```

### 代码模式对比

**老接口**（Messaging / PullAppFileMeta / GetDownloadURL 等）：

```go
func (s *Service) Messaging(ctx context.Context, msg *pbfs.MessagingMeta) (...) {
    im, _ := sfs.ParseFeedIncomingContext(ctx)   // ← 从 sidecar-meta header 构建 Kit
    // 用 im.Kit 干活，bizID 来自 im.Meta.BizID

    // 鉴权：handler 内部自己做
    authorized, _ := s.bll.Auth().Authorize(im.Kit, ra)
}
```

**新接口**（PullKvMeta / GetKvValue / ListApps / GetSingleKvValue 等）：

```go
func (s *Service) PullKvMeta(ctx context.Context, req *pbfs.PullKvMetaReq) (...) {
    kt := kit.FromGrpcContext(ctx)               // ← 从 gRPC 标准 metadata 构建 Kit
    // 用 kt 干活，bizID 来自 req.BizId

    // 鉴权：拦截器已做完，直接取结果
    credential := getCredential(ctx)
}
```

### 对比总结

| | 老接口 | 新接口 |
|---|---|---|
| 接口列表 | Handshake, Messaging, Watch, PullAppFileMeta, GetDownloadURL, GetSingleFileContent | PullKvMeta, GetKvValue, ListApps, AsyncDownload, AsyncDownloadStatus, GetSingleKvValue, GetSingleKvMeta |
| Kit 构建方式 | `ParseFeedIncomingContext(ctx)` → `im.Kit` | `kit.FromGrpcContext(ctx)` → `kt` |
| bizID 来源 | `im.Meta.BizID`（sidecar-meta JSON） | `req.BizId`（protobuf 字段） |
| 鉴权方式 | handler 内部手动调 `s.bll.Auth().Authorize()` | 拦截器 `FeedUnaryAuthInterceptor` 统一处理 |
| 鉴权凭证 | sidecar 身份校验 | Bearer Token（`getCredential(ctx)`） |
| TenantID 读取 | `md.Get("x-bk-tenant-id")` | `md["x-bk-tenant-id"]` |

**关键结论**：两种接口读取 TenantID 的**本质完全一样**——都是从 `ctx` 的 gRPC incoming metadata 中读 `x-bk-tenant-id`。
这个值由拦截器（`FeedEnsureTenantInterceptor` / `FeedStreamEnsureTenantInterceptor`）统一注入，
新老接口只是 Kit 的构建路径和鉴权方式不同，TenantID 的来源完全相同。

---

## 不经过 gRPC 拦截器的 EnsureTenantID 调用

除了 gRPC 拦截器统一处理的部分，还有 **4 处** `EnsureTenantID` 调用不经过 gRPC 拦截器链，
分属两类场景，必须在各自的代码中显式调用。

### 1. HTTP handler（chi 路由，不走 gRPC 拦截器链）

| 文件 | 方法 | 入口 |
|------|------|------|
| `service/service.go` L336 | `DownloadFile` | gRPC-Gateway HTTP 端口 `GET /api/v1/feed/biz/{biz_id}/app/{app}/files/*` |
| `service/rest.go` L34 | `ListFileAppLatestReleaseMetaRest` | 未被路由注册（废代码） |

`DownloadFile` 是挂在 chi 路由上的纯 HTTP handler，走的是 chi 中间件链，不经过 gRPC 拦截器。
Kit 通过 `kit.FromGrpcContext(r.Context())` 构建，没人帮它注入 TenantID，所以在 handler 内部自己调用：

```go
func (s *Service) DownloadFile(w http.ResponseWriter, r *http.Request) {
    kt := kit.FromGrpcContext(r.Context())
    bizID, _ := strconv.Atoi(chi.URLParam(r, "biz_id"))

    // 手动调用，因为不经过 gRPC 拦截器
    if err := s.bll.AppCache().EnsureTenantID(kt, uint32(bizID)); err != nil {
        render.Render(w, r, rest.BadRequest(...))
        return
    }

    // 后续使用 kt（已有 TenantID）
    appID, err := s.bll.AppCache().GetAppID(kt, uint32(bizID), appName)
    ...
}
```

> 注意：gRPC-Gateway 的其他请求（如 `PullKvMeta`、`GetKvValue`）虽然也从 HTTP 端口进入，
> 但通过 `gwMux` 转回了 gRPC RpcPort，会经过 gRPC 拦截器链，**已被统一覆盖**。

**为什么不为 `DownloadFile` 写 chi 中间件统一处理？**
只有这一个 HTTP handler 需要处理租户 ID。为一个 handler 写中间件属于过度设计，
而且中间件需要依赖 URL 参数 `biz_id`，跟具体路由模式耦合。保留在 handler 里，因果关系一行就看完，更简单直接。

### 2. 事件引擎（后台异步任务，没有任何请求上下文）

事件引擎是 feed-server 内部的后台组件，独立于 gRPC/HTTP 请求链路运行。
它通过 `kit.New()` 凭空创建 Kit（没有任何请求上下文和 metadata），只能自己调 `EnsureTenantID`。

共 3 处，对应 3 个不同的触发场景：

#### ① AddSidecar — sidecar 首次连接时匹配 release

**文件**：`bll/eventc/app_event.go` L69-74

**触发时机**：sidecar 通过 `Watch` 建立长连接后，事件引擎为其做首次 release 匹配。

```go
func (ae *appEvent) AddSidecar(currentRelease uint32, sn uint64, subSpec *SubscribeSpec) error {
    me := ae.csm.Add(sn, subSpec)

    kt := kit.New()   // 凭空创建，没有请求上下文
    if err := ae.sch.lc.App.EnsureTenantID(kt, subSpec.InstSpec.BizID); err != nil {
        return err
    }

    // 用 kt 做首次 release 匹配
    matchedRelease, matchedCursor, err := ae.doFirstMatch(kt, subSpec)
    ...
}
```

#### ② eventHandler — 收到发布/变更事件后广播通知

**文件**：`bll/eventc/app_event.go` L158-163

**触发时机**：收到配置发布、应用变更等事件后，逐条处理并广播给所有订阅的 sidecar。

```go
func (ae *appEvent) eventHandler(events []*types.EventMeta) {
    for _, one := range events {
        kt := kit.New()   // 每个事件创建独立 Kit
        if err := ae.sch.lc.App.EnsureTenantID(kt, ae.bizID); err != nil {
            continue
        }

        switch one.Spec.Resource {
        case table.Publish:
            ae.notifyWithApp(kt, one.ID)       // 通知所有 sidecar
        case table.Application:
            ae.handleAppEvent(kt, one)          // 处理应用变更
        case table.RetryInstance:
            ae.notifyWithInstance(kt, one.ID, one.Spec.ResourceUid)  // 重试指定实例
        case table.RetryApp:
            ae.notifyWithApp(kt, one.ID)        // 重试所有失败实例
        }

        ae.cursor.Set(one.ID)
    }
}
```

#### ③ scheduler 重试循环 — 通知失败后定时重试

**文件**：`bll/eventc/scheduler.go` L436-441

**触发时机**：事件通知失败的实例被放入重试队列，定时批量重试。

```go
// scheduler 重试循环
kt := kit.New()
instCount, members := sch.retry.Purge()
for _, one := range members {
    retryKt := kt.Clone()   // 克隆 Kit，每个重试成员独立
    if err := sch.lc.App.EnsureTenantID(retryKt, one.member.InstSpec.BizID); err != nil {
        continue
    }
    sch.notifyEvent(retryKt, one.cursorID, []*member{one.member})
}
```

#### 事件引擎的共同特点

这 3 处有相同的模式：

```go
kt := kit.New()                              // 凭空创建 Kit，TenantID 为空
EnsureTenantID(kt, bizID)                    // 查缓存/RPC 设置 kt.TenantID
// 直接使用这个 kt 对象，不需要 metadata 中转
```

与 gRPC 拦截器不同，事件引擎里**只有一个 Kit 对象**，不存在"拦截器和 handler 各自创建 Kit"的问题，
所以 `EnsureTenantID` 直接写入 `kt.TenantID` 就够了，不需要写回 metadata。

### 全部 EnsureTenantID 调用汇总

```
                     feed-server 中所有 EnsureTenantID 调用
                                      │
               ┌──────────────────────┼──────────────────────┐
               ▼                      ▼                      ▼
        gRPC 拦截器统一处理       HTTP handler              事件引擎
        (interceptor.go)        (service.go)             (eventc/)
               │                      │                      │
        Unary 拦截器 × 1        DownloadFile × 1        AddSidecar × 1
        Stream 拦截器 × 1                               eventHandler × 1
               │                      │               scheduler retry × 1
               │                      │                      │
          覆盖 11 Unary +         1 个 HTTP handler       3 个后台任务
          2 Stream 方法           保留在 handler 内        kit.New() 凭空创建
               │                      │                      │
          通过 metadata 中转      通过 metadata 中转      直接写入 kt.TenantID
          传递给下游 Kit          同一 Kit 直接使用        同一 Kit 直接使用
```

---

## 设计原则

1. **单一入口**：Unary 和 Stream 请求的租户解析分别集中在各自的拦截器中，handler 无需重复调用
2. **metadata 传递**：通过注入 gRPC incoming metadata 跨对象传递 TenantID，避免依赖同一 Kit 实例
3. **非致命错误**：拦截器中 `EnsureTenantID` 失败仅打日志，不阻断请求；具体错误由下游业务逻辑处理
4. **多级缓存**：本地 gcache（进程内、低延迟） → cache-service（分布式、可靠） → 默认值兜底
5. **bizID 来源差异**：Unary 从请求体提取，Stream 从 `sidecar-meta` metadata header 提取（两者 bizID 本质相同）
6. **不过度抽象**：能统一的已统一（gRPC 13 个方法），只有 1 个消费者的场景（HTTP handler、事件引擎）保留显式调用，避免过度设计
