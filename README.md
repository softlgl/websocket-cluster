# websocket-cluster
### golang websocket集群示例
本示例可以构建一个websocket集群，具体涉及和实现思路可以参考[https://www.cnblogs.com/wucy/p/16857160.html](https://www.cnblogs.com/wucy/p/16857160.html)
博客中的示例代码是基于`asp.net core`框架演示的，不过设计思路和实现方式是完全一致的，因此这两份代码可以加入到一个集群内,可跳转至[asp.net core版本实现](https://github.com/softlgl/WebsocketCluster)下载源码，构建集群测试
+ golang版本`go1.19.3 windows/amd64`
+ 开发环境`golang`和`vscode`都支持

关于nginx做websocket集群的配置如下所示，仅供大家参考
```
//上游服务器地址也就是websocket服务的真实地址，其实这里使用ip_hash的方式更合理，这样可以在真实环境中保证同一个客户端多个用户连接分不到一台服务器上
upstream wsbackend {
    server 127.0.0.1:5001;
    server 127.0.0.1:5678;
}

server {
    listen       5000;
    server_name  localhost;

    location ~/chat/{
        //upstream地址
        proxy_pass http://wsbackend;
        proxy_connect_timeout 60s; 
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
        //记得转发避免踩坑
        proxy_set_header Host $host;
        proxy_http_version 1.1; 
        //http升级成websocket协议的头标识
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "Upgrade";
    }
}
```
