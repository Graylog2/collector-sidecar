// (at your option) any later version.
//
// Graylog is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with Graylog.  If not, see <http://www.gnu.org/licenses/>.

package winlogbeat

import (
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/backends"
	"github.com/Graylog2/collector-sidecar/context"
	"path/filepath"
)

const name = "winlogbeat"

var log = common.Log()

func init() {
	if err := backends.RegisterBackend(name, New); err != nil {
		log.Fatal(err)
	}
}

func New(context *context.Ctx) backends.Backend {
	return NewCollectorConfig(context)
}

func (wlbc *WinLogBeatConfig) Name() string {
	return name
}

func (wlbc *WinLogBeatConfig) ExecPath() string {
	execPath := wlbc.Beats.UserConfig.BinaryPath
	if common.FileExists(execPath) != nil {
		log.Fatal("Configured path to collector binary does not exist: " + execPath)
	}

	return execPath
}

func (wlbc *WinLogBeatConfig) ConfigurationPath() string {
	configurationPath := wlbc.Beats.UserConfig.ConfigurationPath
	if !common.IsDir(filepath.Dir(configurationPath)) {
		log.Fatal("Configured path to collector configuration does not exist: " + configurationPath)
	}

	return configurationPath
}

func (wlbc *WinLogBeatConfig) ExecArgs() []string {
	return []string{"-c", wlbc.ConfigurationPath()}
}

func (wlbc *WinLogBeatConfig) ValidatePreconditions() bool {
	return true
}