sijsop - SImple JSOn Protocol
=============================

[![Build Status](https://travis-ci.org/thejerf/sijsop.png?branch=master)](https://travis-ci.org/thejerf/sijsop)

sijsop provides a JSON-based wire or file protocol that provides extremely
good bang-for-the-buck, as well as being very easy to implement in other
languages. See
[the godoc for more information](http://godoc.org/github.com/thejerf/sijsop).

sijsop requires at least Go 1.7.

Code Signing
------------

I will be signing this repository with
the ["jerf" keybase account](https://keybase.io/jerf). If you are viewing
this repository through GitHub, you should see the commits as showing as
"verified" in the commit view.

(Bear in mind that due to the nature of how git commit signing works, there
may be runs of unverified commits; what matters is that the top one is
signed.)

Changlog
--------

1. 1.0.2
  * Add the ability to set the indent on the JSON marshaling process, for
    debugging purposes. (You can set the indent up to make the messages on
    the wire more readable for debugging.)
1. 1.0.1
  * Finish removing the restrictions around a 255-char limit on types.
    I mean, in my opinion you shouldn't have types that long, but there's
    no reason to limit you.
  * Remove the error return from Register for documentation simplicity.
2. 1.0.0
  * [Per ESR](http://esr.ibiblio.org/?p=8254#comment-2202065), this changes
    the protocol to pure text. This is API compatible with 0.9.0, but
    protocol incompatible.
1. 0.9.0
  * This was pulled from some production code and then modified to be
    suitable for public release. The modifications haven't been tested. I
    used the predecessor in production code, and the test cases are
    passing, so I'm reasonably confident this is useful code, but I'm going
    to let it bake in a bit (and take some time later to switch my internal
    code to using this) to be sure before I declare it 1.0.
