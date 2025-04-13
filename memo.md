# デプロイ時にこけたとこ

- cloudflareでDNSを設定
	- VMとcloudflareの間はHTTPで通信するため、SSL/TLSのモードが「フレキシブル」じゃなきゃだめ
	- サブドメインでの運用だったので、ページルールで上記を設定
- setcapでポート80を開放
	- libcap2-binが必要だった
	- /sbin/setcapになるがデフォルトではパスが通っていない
- IPアドレスに対するアクセスを制限
	- VPCネットワークのファイアウォールルールで、IPアドレスをcloudflareのもののみに指定してアクセスを制限
	- 初期設定によっては、デフォルトで全てのIPアドレスからアクセスできるようになっている


# systemctl

```/etc/systemd/system/reminder.service
[Unit]
Description=reminder daemon

[Service]
WorkingDirectory=/home/hoge
ExecStart=/home/hoge/reminder
StandardOutput=append:/home/hoge/log.txt
StandardError=append:/home/hoge/log.txt
Restart=always
Type=simple

[Install]
WantedBy=multi-user.target
```