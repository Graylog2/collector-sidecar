# Copyright (C)  2026 Graylog, Inc.
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the Server Side Public License, version 1,
# as published by MongoDB, Inc.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# Server Side Public License for more details.
#
# You should have received a copy of the Server Side Public License
# along with this program. If not, see
# <http://www.mongodb.com/licensing/server-side-public-license>.
#
# SPDX-License-Identifier: SSPL-1.0

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
          data('COLLECTOR_VERSION_MAJOR') + '.' + data('COLLECTOR_VERSION_MINOR') + '.' + data('COLLECTOR_VERSION_PATCH')
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
