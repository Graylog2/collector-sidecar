require 'sinatra'
require 'json'

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
  config = {'inputs' => []}
  if $inputs[0]
    config['inputs'] << {'name' => 'windows-eventlog', 'properties' => {'Module' => 'im_msvistalog'}}
  end
  if $inputs[1]
    config['inputs'] << {'name' => 'file-log', 'properties' => {'Module' => 'im_file', 'File' => '/var/log/foo'}}
  end
  config.to_json
end
