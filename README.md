# Reminder Application

## 概要

このアプリケーションは、LINE Messaging API を利用したリマインダーボットです。ユーザーは LINE を通じてリマインダーを設定でき、指定した時間に通知を受け取ることができます。

## 機能

*   LINE メッセージによるリマインダーの登録・一覧表示・削除
*   指定時刻に LINE メッセージでリマインダー通知を送信

## 使用技術

### 言語&フレームワーク
- Go
- Echo
- GORM

### データベース
- SQLite

### 開発ツール
- ngrok
- dotenvx
- air
- act

### デプロイ
- Github Actions
- GCP Compute Engine
- Cloudflare

## 実行方法

### ビルド

```bash
make build
```
これにより、`main` という名前の実行ファイルが生成されます。

### 実行

`dotenvx` がインストールされている必要があります。

```bash
make run
```
`.env` ファイルが読み込まれ、アプリケーションが起動します。

### 開発 (ライブリロード)

`dotenvx` と `air` がインストールされている必要があります。`ngrok` と `.env` の `DOMAIN` 設定も推奨されます。

```bash
make watch
```
`air` がファイルの変更を監視し、自動的にアプリケーションを再起動します。`ngrok` がインストールされ、`DOMAIN` が設定されていれば、外部からのアクセス用トンネルも起動します。

## API エンドポイント

*   `GET /`: 動作確認用エンドポイント。 "Hello, World!" を返します。
*   `POST /callback`: LINE Messaging API Webhook エンドポイント。LINE プラットフォームからのイベントを受け取ります。
