#
# ALLHIC: genome scaffolding based on Hi-C data
# Author: Haibao Tang <tanghaibao at gmail dot com>
#

import docopt
import hic/clm
import strutils
import tables

let doc = """
ALLHIC: genome scaffolding based on Hi-C data

Usage:
  allhic partition
  allhic optimize

Options:
  -h --help       Show this screen.
  --version       Show version.
"""

proc optimize_main() =
  var c = initCLMFile("test", "tests/test.clm")
  c.parse()


proc main() =
  let args = docopt(doc, version="ALLHIC 0.7.11")

  if args["partition"]:
    echo "Hello world"

  if args["optimize"]:
    optimize_main()


when isMainModule:
  main()