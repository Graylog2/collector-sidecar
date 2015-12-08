require 'sinatra'
require 'json'
require 'socket'

$inputs = Array.new(2, false)

get '/' do
  erb '<head><style>a.btn { font: bold 11px Arial; text-decoration: none; background-color: #EEEEEE; color: #333333; padding: 2px 6px 2px 6px; border-top: 1px solid #CCCCCC; border-right: 1px solid #333333; border-bottom: 1px solid #333333; border-left: 1px solid #CCCCCC; }</style></head>
        <h2>Graylog Collector Inputs</h2><br>
        <%= if $inputs[0]; "&#x2713;" end %> Windows event log <a href="/input/0" class="btn">Enable Input</a><br>
        <%= if $inputs[1]; "&#x2713;" end %> Log file "/var/log/foo" <a href="/input/1" class="btn">Enable Input</a>'
end

get '/input/:id' do
  idx = params[:id].to_i
  $inputs[idx] = !$inputs[idx]
  redirect "/"
end

get '/configuration' do
  content_type :json
  config = {'inputs' => [], 'outputs' => []}
  if $inputs[0]
    config['inputs'] << {'type' => 'nxlog', 'name' => 'windows-eventlog', 'forward_to' => 'gelf-udp', 'properties' => {'Module' => 'im_msvistalog'}}
  end
  if $inputs[1]
    config['inputs'] << {'type' => 'nxlog', 'name' => 'file-log', 'properties' => {'Module' => 'im_file', 'File' => '"/var/log/foo"'}}
  end
  config['outputs'] << {'type' => 'nxlog', 'name' => 'gelf-udp', 'properties' => {'Module' => 'om_udp', 'Host' => "#{public_address_ipv4}", 'Port' => '12201', 'OutputType' => 'GELF'}}
  config.to_json
end

put '/system/collectors/:id' do
  puts "Collector update: #{request.body.read}"
end

def public_address_ipv4
  addr = Socket.ip_address_list.detect{|intf| intf.ipv4? and !intf.ipv4_loopback? and !intf.ipv4_multicast?}
  return addr.ip_address
end
