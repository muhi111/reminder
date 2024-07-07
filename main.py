from flask import Flask, request, abort
from linebot import (
	LineBotApi, WebhookHandler
)
from linebot.exceptions import (
	InvalidSignatureError
)
from linebot.models import (
	MessageEvent,
	FollowEvent,
	UnfollowEvent,
	PostbackEvent,
	TextMessage,
	TextSendMessage,
	QuickReply,
	QuickReplyButton,
	ButtonsTemplate,
	TemplateSendMessage,
	MessageAction,
	DatetimePickerTemplateAction,
	MessageTemplateAction
)

import os,re,datetime,schedule,time
from textwrap import dedent
import pymysql.cursors
from concurrent.futures import ThreadPoolExecutor

app = Flask(__name__)
line_bot_api = LineBotApi(os.environ["CHANNEL_ACCESS_TOKEN"])
handler = WebhookHandler(os.environ["CHANNEL_SECRET"])

@app.route("/")
def hello_world():
	return "hello world!"

@app.route("/callback", methods=['POST'])
def callback():
	signature = request.headers['X-Line-Signature']
	body = request.get_data(as_text=True)
	app.logger.info("Request body: " + body)
	try:
		handler.handle(body, signature)
		return 'OK'
	except InvalidSignatureError:
		print("Invalid signature. Please check your channel access token/channel secret.")
		abort(400)
		return 'OK'

@handler.add(MessageEvent, message=TextMessage)
def handle_message(event):
	connection = get_connection()
	user_id = event.source.user_id
	text = event.message.text
	text_id = event.message.id
	with connection:
		with connection.cursor() as cursor:
			sql = "SELECT `status` FROM `user_session` WHERE `user_id`=%s"
			cursor.execute(sql, (user_id))
			user_status = cursor.fetchone()
			if text == "使い方":
				how_to_use = """\
					まず、リマインドしたいことを教えてください!
					その後にリマインドして欲しい日時を教えてください!\n
					日時の指定では3つのフォーマットがあります
					・HH:MM
					・mm/dd HH:MM
					・YYYY/mm/dd HH:MM
					明示的に示さなかった部分は現在の値となります
					「一覧」と入力することで現在登録されているリマインドを確認できます
					また、「取り消し」と入力すると登録したリマインドを削除できます
					さらに、「スヌーズ」と入力すると前回リマインドした内容を再び設定できます"""
				line_bot_api.reply_message(
						event.reply_token,
						TextSendMessage(text=dedent(how_to_use),
						quick_reply=QuickReply(items=[
                        QuickReplyButton(action=MessageAction(label="一覧", text="一覧")),
						QuickReplyButton(action=MessageAction(label="取り消し", text="取り消し")),
						QuickReplyButton(action=MessageAction(label="スヌーズ", text="スヌーズ"))
                    ])))
			elif text == "一覧":
				sql = "SELECT `remind_content`, `remind_time` FROM `reminder_content` WHERE `user_id`=%s"
				cursor.execute(sql, (user_id))
				remind_content_list = cursor.fetchall()
				if len(remind_content_list) == 0:
					line_bot_api.reply_message(
						event.reply_token,
						TextSendMessage(text="現在登録されているリマインドはありません"))
				else:
					remind_list = ""
					for content_dict in remind_content_list:
						remind_content = content_dict["remind_content"]
						remind_time_dt = content_dict["remind_time"]
						if remind_time_dt is None:
							continue
						remind_time_sec = remind_time_dt.timestamp()-time.time()
						if remind_time_sec < 0:
							continue
						else:
							# remind_time = re.sub("-","/",remind_time)
							remind_time = remind_time_dt.strftime("%Y/%m/%d %H:%M")
							remind_list += f'{remind_time} \n{remind_content}\n\n'
					if remind_list == "":
						line_bot_api.reply_message(
							event.reply_token,
							TextSendMessage(text="現在登録されているリマインドはありません"))
					else:
						remind_list = re.sub("\n\n$","",remind_list)
						line_bot_api.reply_message(
						event.reply_token,
						TextSendMessage(text=remind_list))
			# リマインダーの登録
			elif user_status["status"] == 0:
				if text == "スヌーズ":
					snooze_remind(event)
				else:
					handle_status_0(event)
			# 時刻の入力
			elif user_status["status"] == 1:
				handle_status_1(event)
			# リマインダーの取り消しの選択
			elif user_status["status"] == 2:
				handle_status_2(event)

@handler.add(FollowEvent)
def handle_follow(event):
	connection = get_connection()
	with connection:
		with connection.cursor() as cursor:
			user_id = event.source.user_id
			send_message(my_user_id, "フォローされました")
			line_bot_api.reply_message(
						event.reply_token,
						[TextSendMessage(text="このbotはデモ版です。個人情報等などは登録しないで下さい。"),
						TextSendMessage(text="また、MessagingAPIの無料枠の関係上、本格的な利用は不可能です。一か月あたり全体で多くとも200回のリマインドしか送れません。"),
						TextSendMessage(text="使い方を知りたい場合「使い方」と入力してください。")])
			sql = "INSERT INTO `user_session` (`user_id`, `status`) VALUES (%s, %s)"
			cursor.execute(sql, (user_id, 0))
			connection.commit()

@handler.add(UnfollowEvent)
def handle_unfollow(event):
	connection = get_connection()
	with connection:
		with connection.cursor() as cursor:
			user_id = event.source.user_id
			send_message(my_user_id, "アンフォローされました")
			sql = "DELETE FROM `reminder_content` WHERE `user_id`=%s"
			cursor.execute(sql, (user_id))
			connection.commit()
			sql = "DELETE FROM `user_session` WHERE `user_id`=%s"
			cursor.execute(sql, (user_id))
			connection.commit()

@handler.add(PostbackEvent)
def on_postback(event):
	user_id = event.source.user_id
	connection = get_connection()
	with connection:
		with connection.cursor() as cursor:
			sql = "SELECT `status` FROM `user_session` WHERE `user_id`=%s"
			cursor.execute(sql, (user_id))
			status = cursor.fetchone()["status"]
			if status == 1:
				remind_time_str = event.postback.params["datetime"]
				remind_time = datetime.datetime.strptime(remind_time_str, "%Y-%m-%dT%H:%M")
				sql = "SELECT `text_id` FROM `user_session` WHERE `user_id`=%s"
				cursor.execute(sql, (user_id))
				text_id = cursor.fetchone()["text_id"]
				sql = "UPDATE `reminder_content` SET `remind_time`=%s WHERE `user_id`=%s AND `text_id`=%s"
				cursor.execute(sql, (remind_time, user_id, text_id))
				connection.commit()
				sql = "UPDATE `user_session` SET `status`=0 WHERE `user_id`=%s"
				cursor.execute(sql, (user_id))
				connection.commit()
				schedule.every(remind_time.timestamp()-time.time()).seconds.do(remind,text_id,user_id)
				line_bot_api.reply_message(
					event.reply_token,
					TextSendMessage(text=f'登録できました!\n{remind_time.strftime("%Y/%m/%d %H:%M")}にリマインドします'))
			else:
				line_bot_api.reply_message(
					event.reply_token,
					TextSendMessage(text=f"無効な日付選択アクションです"))

def handle_status_0(event):
	connection = get_connection()
	user_id = event.source.user_id
	text = event.message.text
	text_id = event.message.id
	with connection:
		with connection.cursor() as cursor:
			if text == "取り消し":
				sql = "SELECT `remind_content`, `remind_time` FROM `reminder_content` WHERE `user_id`=%s"
				cursor.execute(sql, (user_id))
				remind_content_list = cursor.fetchall()
				if len(remind_content_list) == 0:
					line_bot_api.reply_message(
						event.reply_token,
						TextSendMessage(text="現在登録されているリマインドはありません"))
				else:
					remind_list = ""
					for content_dict in remind_content_list:
						remind_content = content_dict["remind_content"]
						remind_time_dt = content_dict["remind_time"]
						if remind_time_dt.timestamp()-time.time() < 0:
							continue
						else:
							remind_time = remind_time_dt.strftime("%Y/%m/%d %H:%M")
							remind_list += f'{remind_time} \n{remind_content}\n\n'
					if remind_list == "":
						line_bot_api.reply_message(
						event.reply_token,
						TextSendMessage(text="現在登録されているリマインドはありません"))
					else:
						remind_list = re.sub("\n\n$","",remind_list)
						line_bot_api.reply_message(
							event.reply_token,
							[TextSendMessage(text=remind_list),
							TextSendMessage(text="どのリマインドを取り消すか入力して下さい"),
							TextSendMessage(text="また、同じ内容のリマインドはすべて削除されるので注意して下さい"),
							TextSendMessage(text="やめたい場合は「キャンセル」を押して下さい",
							quick_reply=QuickReply(items=[
                    	    QuickReplyButton(action=MessageAction(label="キャンセル", text="キャンセル"))
                    	]))])
						sql = "UPDATE `user_session` SET `status`=2 WHERE `user_id`=%s"
						cursor.execute(sql, (user_id))
						connection.commit()
			else:
				sql = "INSERT INTO `reminder_content` (`user_id`, `remind_content`, `remind_time`, `text_id`) VALUES (%s, %s, NULL, %s)"
				cursor.execute(sql, (user_id, text, text_id))
				connection.commit()
				now = datetime.datetime.now()
				line_bot_api.reply_message(
					event.reply_token,
					TemplateSendMessage(
						alt_text="日時選択",
						template=ButtonsTemplate(
							text="リマインド日時を選択",
							actions=[
								DatetimePickerTemplateAction(
									label="日時選択",
									data="id",
									mode="datetime",
									initial=(now + datetime.timedelta(hours=1)).strftime("%Y-%m-%dT%H:00"),
									min=now.strftime("%Y-%m-%dT%H:%M"),
									max=f"{now.year + 1}-12-31T23:59")]),
						quick_reply=QuickReply(items=[
							QuickReplyButton(action=MessageAction(label="キャンセル", text="キャンセル"))])))
				update_status(user_id,1,text_id)

def handle_status_1(event):
	connection = get_connection()
	user_id = event.source.user_id
	text = event.message.text
	text_id = event.message.id
	with connection:
		with connection.cursor() as cursor:
			if text == "キャンセル":
				sql = "SELECT `text_id` FROM `user_session` WHERE `user_id`=%s"
				cursor.execute(sql, (user_id))
				uuid = cursor.fetchone()
				sql = "DELETE FROM `reminder_content` WHERE `user_id`=%s AND `text_id`=%s"
				cursor.execute(sql, (user_id, uuid["text_id"]))
				connection.commit()
				update_status(user_id,0,None)
				line_bot_api.reply_message(
						event.reply_token,
						TextSendMessage(text="キャンセルしました"))
			elif text == "再送":
				now = datetime.datetime.now()
				line_bot_api.reply_message(
					event.reply_token,
					TemplateSendMessage(
						alt_text="日時選択",
						template=ButtonsTemplate(
							text="リマインド日時を選択",
							actions=[
								DatetimePickerTemplateAction(
									label="日時選択",
									data="id",
									mode="datetime",
									initial=(now + datetime.timedelta(hours=1)).strftime("%Y-%m-%dT%H:00"),
									min=now.strftime("%Y-%m-%dT%H:%M"),
									max=f"{now.year + 1}-12-31T23:59")]),
						quick_reply=QuickReply(items=[
							QuickReplyButton(action=MessageAction(label="キャンセル", text="キャンセル"))])))
			else:
				line_bot_api.reply_message(
						event.reply_token,
						[TextSendMessage(text="日時選択アクションからリマインドする日時を選択してさい。"),
						TextSendMessage(text="再送して欲しい場合は「再送」を、キャンセルしたい場合は「キャンセル」を押してください。",
										quick_reply=QuickReply(items=[
											QuickReplyButton(action=MessageAction(label="キャンセル", text="キャンセル")),
											QuickReplyButton(action=MessageAction(label="再送", text="再送"))]))])

def handle_status_2(event):
	connection = get_connection()
	user_id = event.source.user_id
	text = event.message.text
	with connection:
		with connection.cursor() as cursor:
			if text == "キャンセル":
				update_status(user_id,0,None)
				line_bot_api.reply_message(
						event.reply_token,
						TextSendMessage(text="キャンセルしました"))
			else:
				sql = "SELECT * FROM `reminder_content` WHERE `user_id`=%s AND `remind_content`=%s"
				cursor.execute(sql, (user_id,text))
				remind_content_list = cursor.fetchall()
				if len(remind_content_list) == 0:
					line_bot_api.reply_message(
						event.reply_token,
						TextSendMessage(text="存在しないリマインドです\nやめたい場合は「キャンセル」を押して下さい",
						quick_reply=QuickReply(items=[
                        QuickReplyButton(action=MessageAction(label="キャンセル", text="キャンセル"))
                    ])))
				else:
					sql = "DELETE FROM `reminder_content` WHERE `user_id`=%s AND `remind_content`=%s"
					cursor.execute(sql, (user_id, text))
					connection.commit()
					sql = "UPDATE `user_session` SET `status`=0, `text_id`=NULL WHERE `user_id`=%s"
					cursor.execute(sql, (user_id))
					connection.commit()
					line_bot_api.reply_message(
						event.reply_token,
						TextSendMessage(text="取り消しが出来ました!"))

def get_connection():
	return pymysql.connect(host=os.environ["NS_MARIADB_HOSTNAME"],
							port=int(os.environ["NS_MARIADB_PORT"]),
							user=os.environ["NS_MARIADB_USER"],
                            password=os.environ["NS_MARIADB_PASSWORD"],
                            db=os.environ["NS_MARIADB_DATABASE"],
                            charset='utf8mb4',
                            cursorclass=pymysql.cursors.DictCursor)

def update_status(user_id,status,text_id):
	connection = get_connection()
	with connection:
		with connection.cursor() as cursor:
			sql = "UPDATE `user_session` SET `status`=%s, `text_id`=%s WHERE `user_id`=%s"
			cursor.execute(sql, (status,text_id,user_id))
			connection.commit()

def snooze_remind(event):
	connection = get_connection()
	user_id = event.source.user_id
	text_id = event.message.id
	with connection:
		with connection.cursor() as cursor:
			sql = "SELECT `last_notify_text_id` FROM `user_session` WHERE `user_id`=%s"
			cursor.execute(sql, (user_id))
			last_notify_text_id_for_snooze = cursor.fetchone()
			last_notify_text_id_for_snooze = last_notify_text_id_for_snooze["last_notify_text_id"]
			if last_notify_text_id_for_snooze is None:
				line_bot_api.reply_message(
					event.reply_token,
					TextSendMessage(text="リマインド履歴が確認出来なかったため、スヌーズに失敗しました"))
			else:
				sql = "SELECT `remind_content` FROM `reminder_content` WHERE `user_id`=%s AND `text_id`=%s"
				cursor.execute(sql, (user_id,last_notify_text_id_for_snooze))
				remind_content_for_snooze = cursor.fetchone()
				if remind_content_for_snooze is None:
					line_bot_api.reply_message(
						event.reply_token,
						TextSendMessage(text="何らかの事情で履歴を遡れなかったため、スヌーズに失敗しました"))
				else:
					remind_content_for_snooze = remind_content_for_snooze["remind_content"]
					for job in schedule.jobs:
						if all(x in str(job) for x in ("delete_history", user_id, last_notify_text_id_for_snooze)):
							schedule.cancel_job(job)
							break
					sql = "DELETE FROM `reminder_content` WHERE `user_id`=%s AND `text_id`=%s"
					cursor.execute(sql, (user_id, last_notify_text_id_for_snooze))
					connection.commit()
					sql = "INSERT INTO `reminder_content` (`user_id`, `remind_content`, `remind_time`, `text_id`) VALUES (%s, %s, NULL, %s)"
					cursor.execute(sql, (user_id, remind_content_for_snooze, text_id))
					connection.commit()
					update_status(user_id,1,text_id)
					now = datetime.datetime.now()
					line_bot_api.reply_message(
							event.reply_token,
							[TextSendMessage(text=f"「{remind_content_for_snooze}」のスヌーズに成功しました。"),
							TemplateSendMessage(
								alt_text="日時選択",
								template=ButtonsTemplate(
									text="リマインド日時を選択",
									actions=[
										DatetimePickerTemplateAction(
											label="日時選択",
											data="id",
											mode="datetime",
											initial=(now + datetime.timedelta(hours=1)).strftime("%Y-%m-%dT%H:00"),
											min=now.strftime("%Y-%m-%dT%H:%M"),
											max=f"{now.year + 1}-12-31T23:59")]),
								quick_reply=QuickReply(items=[
									QuickReplyButton(action=MessageAction(label="キャンセル", text="キャンセル"))]))])

def delete_history(user_id,text_id):
	connection = get_connection()
	with connection:
		with connection.cursor() as cursor:
			sql = "DELETE FROM `reminder_content` WHERE `user_id`=%s AND `text_id`=%s"
			cursor.execute(sql, (user_id, text_id))
			connection.commit()
	return schedule.CancelJob

def remind(text_id,user_id):
	connection = get_connection()
	with connection:
		with connection.cursor() as cursor:
			sql = "SELECT `remind_content` FROM `reminder_content` WHERE `user_id`=%s AND `text_id`=%s"
			cursor.execute(sql, (user_id,text_id))
			remind_content = cursor.fetchone()
			line_bot_api.push_message(user_id, TextSendMessage(text=f'「{remind_content["remind_content"]}」の時間です',
			quick_reply=QuickReply(items=[
                        QuickReplyButton(action=MessageAction(label="スヌーズ", text="スヌーズ"))
                    ])))
			schedule.every(86400).seconds.do(delete_history,user_id,text_id)
			sql = "UPDATE `user_session` SET `last_notify_text_id`=%s WHERE `user_id`=%s"
			cursor.execute(sql, (text_id,user_id))
			connection.commit()
	return schedule.CancelJob

def send_message(user_id,message_text):
	line_bot_api.push_message(user_id, TextSendMessage(text=message_text))

def app_run():
	app.run(host="0.0.0.0", port=8080)

def schedule_func():
	while True:
		print("schedule working...")
		print(schedule.get_jobs())
		schedule.run_pending()
		time.sleep(5)

def re_schedule():
	connection = get_connection()
	with connection:
		with connection.cursor() as cursor:
			sql = "SELECT * FROM `reminder_content`"
			cursor.execute(sql)
			remind_list = cursor.fetchall()
			for remind_dict in remind_list:
				remind_time_dt = remind_dict["remind_time"]
				if remind_time_dt is None:
					continue
				user_id = remind_dict["user_id"]
				text_id = remind_dict["text_id"]
				remind_time_sec = remind_time_dt.timestamp()-time.time()
				if remind_time_sec < 0:
					sql = "DELETE FROM `reminder_content` WHERE `user_id`=%s AND `text_id`=%s"
					cursor.execute(sql, (user_id, text_id))
					connection.commit()
				else:
					schedule.every(remind_time_sec).seconds.do(remind,text_id,user_id)

if __name__ == "__main__":
	my_user_id = os.environ["MY_USER_ID"]
	re_schedule()
	send_message(my_user_id, "デプロイが完了しました")
	with ThreadPoolExecutor(2) as executor:
		executor.submit(app_run)
		executor.submit(schedule_func)