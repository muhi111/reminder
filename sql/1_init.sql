CREATE TABLE `user_session` (
    `user_id` VARCHAR(255) NOT NULL,
	`text_id` VARCHAR(255),
	`last_notify_text_id` VARCHAR(255),
    `status` INT NOT NULL
);

CREATE TABLE `reminder_content` (
    `user_id` VARCHAR(255) NOT NULL,
    `remind_content` TEXT,
    `remind_time` DATETIME,
    `text_id` VARCHAR(255) 
);