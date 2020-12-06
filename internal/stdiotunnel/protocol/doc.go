/*
Package protocol - MIT License Copyright (c) 2020, Rectcircle. All rights reserved.

This is the project core code - protocol implementation

1. Segment - attach control protocol header to data chunk

2. Bridge - handle Segment

3. Tunnel - virtual connection

Architecture diagram:
                       Client                                                 Server
                                                                         +--------------+
                +----------------+                                       |              |
                |                |                                       | bridge.serve |
            +-->| tunnel.forward |-------------------------------------->| for segment  |-----+
            |   |                |                                       |    ...       |     |
            |   +----------------+                                       |              |     |
    --------+                                                            +--------------+     |
    <-------+                                                                                 +-------->
            |    +--------------+                                                             +---------
            |    |              |                                       +----------------+    |
            |    | bridge.serve |                                       |                |    |
            +----| for segment  |<--------------------------------------| tunnel.forward |<---+
                 |    ...       |                                       |                |
                 |              |                                       +----------------+
                 +--------------+
*/
package protocol
