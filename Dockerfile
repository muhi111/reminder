# pythonの公式イメージをベースにする
FROM python:latest

# 作業ディレクトリを指定
WORKDIR /app

# ホストのrequirements.txtをコンテナの作業ディレクトリにコピー
COPY requirements.txt .

# pipでrequirements.txtに記載されたパッケージをインストール
RUN pip install --no-cache-dir -r requirements.txt

# ホストのファイルをコンテナの作業ディレクトリにコピー
COPY . .

# コンテナが起動した時に実行されるコマンド
CMD ["python", "-u", "main.py"]
