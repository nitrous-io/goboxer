package goboxer

import (
	"text/template"
)

var UpstartConfSettings = map[string]string{
	"ActionUser":         "action",
	"PartsInitLog":       ".parts-init.log",
	"NitrousboxrcFile":   ".nitrousboxrc",
	"NitrousboxrcSample": ".nitrousboxrc.sample",
	"NitrousboxrcLog":    ".nitrousboxrc.log",
}

const PartsInitConfTemplate = `
# parts-init
description "runs parts init as {{.ActionUser}} user"

task
start on (net-device-up
          and local-filesystems
          and runlevel [2345])

env HOME="/home/{{.ActionUser}}"

setuid {{.ActionUser}}
setgid {{.ActionUser}}

console log

chdir /home/{{.ActionUser}}

exec parts init - >> /home/{{.ActionUser}}/{{.PartsInitLog}}
`

// TODO: this Upstart conf does not enforce timeouts on .nitrousboxrc execution
// time
const RunNitrousboxrcConfTemplate = `
# run-nitrousboxrc
description "runs /home/{{.ActionUser}}/{{.NitrousboxrcFile}} as {{.ActionUser}} user"

task
start on stopped parts-init

env HOME="/home/{{.ActionUser}}"
env BOXRC="/home/{{.ActionUser}}/{{.NitrousboxrcFile}}"
env BOXRC_LOG="/home/{{.ActionUser}}/{{.NitrousboxrcLog}}"
export HOME
export BOXRC
export BOXRC_LOG

setuid {{.ActionUser}}
setgid {{.ActionUser}}

kill timeout 5 # note: doesn't actually time out the $BOXRC run
console log

chdir /home/{{.ActionUser}}

pre-start script
  [ ! -x $BOXRC ] && { stop; exit 0; }
  echo "$(date +'%F %T') : Starting $BOXRC" >> $BOXRC_LOG
end script

exec $BOXRC >> $BOXRC_LOG 2>&1

post-stop script
  [ ! -x $BOXRC ] && exit 0
  echo "$(date +'%F %T') : Finished $BOXRC" >> $BOXRC_LOG
end script
`

const NitrousboxrcSampleTemplate = `#!/bin/bash

# Note: You can also use #!/bin/sh or #!/bin/zsh as the first line.

# Use {{.NitrousboxrcFile}} to run commands (start daemons, etc) when your box is
# started/restarted.
# The results of running this script will be appended to
# /home/{{.ActionUser}}/{{.NitrousboxrcLog}} .

# To use this file, run the following commands in your terminal:
#   cp ~/{{.NitrousboxrcSample}} ~/{{.NitrousboxrcFile}}
#   chmod +x ~/{{.NitrousboxrcFile}}

# The line below is recommended so that any errors will stop the script.
set -e

# Add your commands here...
`

var (
	PartsInitConfTemplateCompiled       *template.Template
	RunNitrousboxrcConfTemplateCompiled *template.Template
	NitrousboxrcSampleTemplateCompiled  *template.Template
)

func init() {
	PartsInitConfTemplateCompiled = template.Must(template.New("parts-init.conf").Parse(PartsInitConfTemplate))
	RunNitrousboxrcConfTemplateCompiled = template.Must(template.New("run-nitrousboxrc.conf").Parse(RunNitrousboxrcConfTemplate))
	NitrousboxrcSampleTemplateCompiled = template.Must(template.New("nitrousboxrc.sample").Parse(NitrousboxrcSampleTemplate))
}
