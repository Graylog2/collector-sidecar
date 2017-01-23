require 'json'

module FPM
  module Cookery
    class Recipe
      class RecipeData
        def initialize(recipe)
          @json = JSON.parse(File.read(File.expand_path('../../versions.json', __FILE__)))
          @recipe = recipe
        end

        def version
          data('version')
        end

        def suffix
          data('suffix')
        end

        def revision
          if data('suffix')
            data('revision').to_s + data('suffix').gsub(/^-/, '.')
          else
            data('revision')
          end
        end

        def data(key)
          raise "Missing value for #{key} in versions.json" if !@json.key?(key)
          @json[key]
        end
      end

      def self.data
        RecipeData.new(self)
      end

      def data
        self.class.data
      end
    end
  end
end
