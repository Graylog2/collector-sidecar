require 'sinatra'
require 'json'

get '/configuration' do
  content_type :json
  config = {'inputs' => [{'name' => 'windows-eventlog', 'properties' => {'Module' => 'im_msvistalog'}}]}
  config.to_json
end
