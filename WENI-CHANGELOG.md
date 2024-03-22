1.18.0-mailroom-7.1.22
----------
 * Adjust chatgpt prompt for product list
 * Perform cart simulation for product list using postal code

1.17.3-mailroom-7.1.22
----------
 * /mr/health do health check for redis, database, sentry and s3

1.17.2-mailroom-7.1.22
----------
 * handling tickets/utils SendReply msg return
 * twilioflex open ticket handling to close cleanup flex resource if webhook conf fail

1.17.1-mailroom-7.1.22
----------
 * Improve prompt for chatGPT in product listings

1.17.0-mailroom-7.1.22
----------
 * Fix errors for duplicate products

1.16.0-mailroom-7.1.22
----------
 * Implement changes to preserve the insertion order in the product list

1.15.0-mailroom-7.1.22
----------
 * Update goflow version to v0.6.3-goflow-0.144.3

1.14.0-mailroom-7.1.22
----------
 * Update goflow version to v0.6.2-goflow-0.144.3

1.13.1-mailroom-7.1.22
----------
 * fix wenichats sending history when opening ticket, adding a margin time in the selection parameter to avoid omitting the first message that was created before the flowrun

1.13.0-mailroom-7.1.22
----------
 * Implement the nfm_reply field in input

1.12.0-mailroom-7.1.22
----------
 * Add handle for weniGPTCalled type events
 * Revert "Merge pull request #70 from weni-ai/fix/wenichats-send-history"

1.11.2-mailroom-7.1.22
----------
 * Fix sellerId logic for intelligent vtex search request

1.11.1-mailroom-7.1.22
----------
 * fix wenichats send history on open ticket based on first message and flowrun events

1.11.0-mailroom-7.1.22
----------
 * Update goflow version to v0.5.2-goflow-0.144.3

1.10.0-mailroom-7.1.22
----------
 * Update goflow version to v0.5.1-goflow-0.144.3

1.9.1-mailroom-7.1.22
----------
 * Change org config data name

1.9.0-mailroom-7.1.22
----------
 * Add orgContext asset

1.8.0-mailroom-7.1.22
----------
 * Vtex search support with sellerId
 * [FLOWS-285] - Add contact in httplog model

1.7.3-mailroom-7.1.22
----------
 * Add support for searching for Vtex products

1.7.2-mailroom-7.1.22
----------
 * Remove duplicate products for catalog messages

1.7.1-mailroom-7.1.22
----------
 * Tweaks for close ticket behaviour to not delete unfired campaign contact events

1.7.0-mailroom-7.1.22
----------
 * Catalog message support

1.6.14-mailroom-7.1.22
----------
 * Update goflow version

1.6.13-mailroom-7.1.22
----------
 * Fix/order trigger resume

1.6.12-mailroom-7.1.22
----------
 * Changing function names for zeroshot variables

1.6.11-mailroom-7.1.22
----------
 * Add new environment variables for zeroshot api

1.6.10-mailroom-7.1.22
----------
 * Fix twilioflex panic on fetch media error

1.6.9-mailroom-7.1.22
----------
 * Wenichats open room receiving defined custom fields of body is properly configured or all fields if not

1.6.8-mailroom-7.1.22
----------
 * Fix zendesk ticketer client users endpoints

1.6.7-mailroom-7.1.22
----------
 * Relates a zendesk user with external id equal to contact uuid when opening zendesk ticket

1.6.6-mailroom-7.1.22
----------
 * Increase tests coverage in web module

1.6.5-mailroom-7.1.22
----------
 * Add support to receive order on input and tigger and any metadata object in trigger

1.6.4-mailroom-7.1.22
----------
 * Update goflow version for v0.1.1-goflow-0.144.3

1.6.3-mailroom-7.1.22
----------
 * Add close room webhook in parameters for RocketChat

1.6.2-mailroom-7.1.22
----------
 * Add email field to params 

1.6.1-mailroom-7.1.22
----------
 * Add ChatGPT configs for temperature and top_p

1.6.0-mailroom-7.1.22
----------
 * Add ChatGPT external service

1.5.8-mailroom-7.1.22
----------
 * Support for trigger.params in Msg events

1.5.7-mailroom-7.1.22
----------
 * Remove returned ticket in case of reopening for zendesk

1.5.6-mailroom-7.1.22
----------
 * Support for reopening zendesk ticket with body ID

1.5.5-mailroom-7.1.22
----------
 * Add support for sending webhook parameters in trigger.params to Zendesk

1.5.4-mailroom-7.1.22
----------
 * Fix goflow version command 

1.5.3-mailroom-7.1.22
----------
 * Send is_anon field when opening room in wenichats

1.5.2-mailroom-7.1.22
----------
 * Remove topups to fix tests

1.5.1-mailroom-7.1.22
----------
 * use contact id for name on open wenichats ticket if name is empty

1.5.0-mailroom-7.1.22
----------
 * Support for the new Omie external service

1.4.18-mailroom-7.1.22
----------
 * Fix wenichats send media #118
 * Ordering Twilioflex msg history and send each as a separated message #117

1.4.17-mailroom-7.1.22
----------
 * Send message attribute to Zendesk on attachment submissions #115
 * Tweak wenichats integration open room to pass FlowUUID and contact groups #113 & #114
 * Fix FetchFileWithMaxSize #113

1.4.16-mailroom-7.1.22
----------
 * Added wenichats webhook media body bytes limits #111

1.4.15-mailroom-7.1.22
----------
 * Send chatbot history on Twilio in just one message #109

1.4.14-mailroom-7.1.22
----------
 * Added config for flow start batch timeout #107

1.4.13-mailroom-7.1.22
----------
 * Add Domain to File URL for Zendesk #105

1.4.12-mailroom-7.1.22
----------
 * Fix file endpoint for Zendesk #103

1.4.11-mailroom-7.1.22
----------
 * Add support for sending contact language in messages for WAC and WA #101

1.4.10-mailroom-7.1.22
----------
 * Fix submitting tags and custom fields for Zendesk tickets #99

1.4.9-mailroom-7.1.22
----------
 * add contact urn field to wenichats room creation params #97

1.4.8-mailroom-7.1.22
----------
 * Fix twilio flex messages history #95

1.4.7-mailroom-7.1.22
----------
 * Fix twilio flex media creation content-type param #93

1.4.6-mailroom-7.1.22
----------
 * Fix tag registration, custom_fields and ticket closing in Zendesk #91

1.4.5-mailroom-7.1.22
----------
 * Add Ticket Fields for Zendesk #86
 * twilio flex detect and setup media on create media type  #87
 * twilio flex open ticket can set preferred flexflow from body json field flex_flow_sid #88
 * Swap targets for webhooks in Zendesk #89

1.4.4-mailroom-7.1.22
----------
 * wenichats open ticket with contact fields as default in addition to custom fields

1.4.3-mailroom-7.1.22
----------
 * fix twilio flex contact echo msgs from webhook

1.4.2-mailroom-7.1.22
----------
 * twilio flex support extra fields
 * twilio flex has Header X-Twilio-Webhook-Enabled=True on send msg

1.4.1-mailroom-7.1.22
----------
 * wenichats ticketer support custom fields

1.4.0-mailroom-7.1.22
----------
 * Add wenichats ticketer integration

1.3.3-mailroom-7.1.22
----------
 * Fix contacts msgs query

1.3.2-mailroom-7.1.22
----------
* Replace gocommon v1.16.2 with version v1.16.2-weni compatible with Teams channel

1.3.1-mailroom-7.1.22
----------
 * Replace gocommon for one with slack bot channel urn

1.3.0-mailroom-7.1.22
----------
 * Merge nyaruka tag v7.1.22 into weni 1.2.1-mailroom-7.0.1 and resolve conflicts.

1.2.1-mailroom-7.0.1
----------
 * Tweak ticketer Twilio Flex to allow API key authentication

1.2.0-mailroom-7.0.1
----------
 * Add ticketer Twilio Flex

1.1.0-mailroom-7.0.1
----------
 * Update gocommon to v1.15.1

1.0.0-mailroom-7.0.1
----------
 * Update Dockerfile to go 1.17.5
 * Fix ivr cron retry calls
 * More options in "wait for response". 15, 30 and 45 seconds
 * Support to build Docker image
