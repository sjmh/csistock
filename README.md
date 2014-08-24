
CSI Stock Checker
-----------------

This is a simple script for page scraping the http://www.coolstuffinc.com site for your wishlist and informing you via Boxcar to your mobile phone when certain events ( low stock, out of stock, back in stock ) occur.

### Requirements

You'll need:

  - A mobile device
  - The BoxCar app on said mobile device
  - A wishlist on http://www.coolstuffinc.com

### Setup

To configure this script, you'll need two environment variables.

  - BOXCAR_TOKEN: This envvar should be set to your access token.  You can find this in the settings of the BoxCar app on your mobile device.
  - CSI_WISHLIST: This envvar should be set to the URL of your wishlist.  You can find this at the bottom of your wishlist.

  These envvars can be specified on the command line as well, e.g.:

  `BOXCAR_TOKEN=abcde12345 CSI_WISHLIST=http://www.coolstuffinc.com/user_wishlist.php?id=987654321 ./csistock`

  Note: You'll need to escape the ? in the CSI_WISHLIST URL if you do it via command line.

### Running

Once you've got the pre-reqs and setup, you just need to fire off the script.  It will poll the site every 5 minutes and report any events back to your phone.  You can background the process if you'd like or setup a watchdog via cron to ensure it's always running.


