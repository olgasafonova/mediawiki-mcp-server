<?php
/**
 * MediaWiki LocalSettings.php for integration testing
 *
 * This configuration enables the API and allows bot access for testing.
 */

# General settings
$wgSitename = "TestWiki";
$wgMetaNamespace = "TestWiki";
$wgServer = "http://localhost:8080";
$wgScriptPath = "";
$wgArticlePath = "/wiki/$1";

# Database settings (SQLite)
$wgDBtype = "sqlite";
$wgDBserver = "";
$wgDBname = "mediawiki";
$wgDBuser = "";
$wgDBpassword = "";
$wgSQLiteDataDir = "/var/www/data";

# Secret key - generate new one for production
$wgSecretKey = "test-secret-key-do-not-use-in-production-12345678901234567890";
$wgAuthenticationTokenVersion = "1";

# Site language
$wgLanguageCode = "en";

# License
$wgRightsPage = "";
$wgRightsUrl = "";
$wgRightsText = "";
$wgRightsIcon = "";

# Disable file cache for testing
$wgEnableUploads = true;
$wgUseFileCache = false;

# API settings - enable for testing
$wgEnableAPI = true;
$wgEnableWriteAPI = true;

# Allow anonymous editing (useful for some tests)
$wgGroupPermissions['*']['edit'] = true;
$wgGroupPermissions['*']['createpage'] = true;

# Bot permissions
$wgGroupPermissions['bot']['bot'] = true;
$wgGroupPermissions['bot']['autoconfirmed'] = true;
$wgGroupPermissions['bot']['noratelimit'] = true;

# User permissions for testing
$wgGroupPermissions['user']['edit'] = true;
$wgGroupPermissions['user']['createpage'] = true;
$wgGroupPermissions['user']['upload'] = true;
$wgGroupPermissions['user']['reupload'] = true;
$wgGroupPermissions['user']['move'] = true;
$wgGroupPermissions['user']['delete'] = true;
$wgGroupPermissions['user']['undelete'] = true;

# Disable rate limiting for tests
$wgRateLimits = [];

# Debug settings for testing
$wgShowExceptionDetails = true;
$wgShowDBErrorBacktrace = true;

# Allow shorter passwords for testing
$wgMinimalPasswordLength = 1;

# Disable email for testing
$wgEnableEmail = false;
$wgEnableUserEmail = false;

# Default skin
$wgDefaultSkin = "vector";
wfLoadSkin( 'Vector' );
