
中心化的配置中心

业余爱好项目，功能实现后还没有做代码的优化，名字空间的修改等，所以代码还是比较粗糙的，假如要用，可以先优化一下代码后再使用


使用方式

1. 修改GAegentMain 文件中中心配置服务器的ip和端口，编译后部署到宿主机器然后启动
2. 修改ServerMain 中关于配置文件的位置，编译后部署到中心配置服务器，启动
3. 如下代码是php获取配置的方式，可以封装后使用


<code lange=php>
$fd = stream_socket_client("unix:///tmp/GConfAgent.sock");
fwrite($fd,"test001/a");
var_dump(trim(fgets($fd)));
</code>

欢迎一起交流心得 Tel：15911185633

