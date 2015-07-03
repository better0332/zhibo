DROP TABLE IF EXISTS `live_proxy`;
CREATE TABLE `live_proxy` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `category` tinyint(3) unsigned NOT NULL,
  `unique_md5` char(32) NOT NULL,
  `site` varchar(255) NOT NULL,
  `title` varchar(255) NOT NULL,
  `online` int(10)  unsigned NOT NULL,
  `status` tinyint(3) unsigned NOT NULL DEFAULT 1,
  `start_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `end_time` timestamp NOT NULL DEFAULT '0000-00-00 00:00:00',
  `cache_size` bigint(20) unsigned NOT NULL,
  `service_size` bigint(20) unsigned NOT NULL,
  PRIMARY KEY (`id`),
  KEY `status` (`status`)
) ENGINE=MyISAM DEFAULT CHARSET=utf8;