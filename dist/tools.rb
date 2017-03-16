require 'json'

module FPM
  module Cookery
    class Recipe
      class RecipeData
        def initialize(recipe)
          version_mk = File.read(File.expand_path('../../version.mk', __FILE__))
          @version = Hash[*version_mk.gsub(/"/,'').gsub(/^.*=\s*\n^/, '').split(/\s*[\n=]\s*/)]
          @recipe = recipe
        end

        def version
          data('COLLECTOR_VERSION')
        end

        def suffix
          data('COLLECTOR_VERSION_SUFFIX')
        end

        def revision
          if @version.key?('COLLECTOR_VERSION_SUFFIX')
            data('COLLECTOR_REVISION').to_s + data('COLLECTOR_VERSION_SUFFIX').gsub(/^-/, '.')
          else
            data('COLLECTOR_REVISION')
          end
        end

        def data(key)
          raise "Missing value for #{key} in version.mk" if !@version.key?(key)
          @version[key]
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
