针对sina sports、cntv。将视频直播流请求重定向到引擎，由引擎代理访问视频源服务器，并做内存缓存。为了优化性能，使用了自定义的队列结构，减少了锁的使用，必要时用原子锁处理队列。针对flv的媒体流，做了关键帧解析